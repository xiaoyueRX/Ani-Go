"""用真实的 qBittorrent 验收 RSS、选集、入库及重启后的独立做种。"""

import argparse
import base64
from contextlib import closing, contextmanager, ExitStack
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import time
from urllib.error import URLError
from urllib.parse import urlencode
from urllib.request import build_opener, HTTPCookieProcessor, ProxyHandler, Request

from test_platform_runtime import HOST, start
from torrent_fixture import create_pack_subscription, free_port, movie_feed, NAMES, PAYLOADS, TITLE, verify_library, wait_for, web_seed


class QB:
    def __init__(self, program, profile, downloads):
        self.program = program
        self.profile = profile
        self.downloads = downloads
        self.web_port = free_port()
        self.peer_port = free_port()
        self.password = secrets.token_urlsafe(24)
        self.opener = build_opener(ProxyHandler({}), HTTPCookieProcessor(http.cookiejar.CookieJar()))
        self.url = f"http://{HOST}:{self.web_port}"
        salt = os.urandom(16)
        derived = hashlib.pbkdf2_hmac("sha512", self.password.encode(), salt, 100000)
        auth = ":".join(base64.b64encode(value).decode() for value in (salt, derived))
        extension = ".ini" if sys.platform in ("win32", "darwin") else ".conf"
        config = profile / "qBittorrent" / "config" / ("qBittorrent" + extension)
        config.parent.mkdir(parents=True)
        config.write_text(
            "[BitTorrent]\nSession\\DHTEnabled=false\nSession\\PeXEnabled=false\nSession\\LSDEnabled=false\n"
            f"Session\\InterfaceAddress={HOST}\nSession\\Port={self.peer_port}\n"
            "[LegalNotice]\nAccepted=true\n[Preferences]\nAdvanced\\updateCheck=false\nWebUI\\Enabled=true\n"
            f"WebUI\\Address={HOST}\nWebUI\\Port={self.web_port}\nWebUI\\LocalHostAuth=true\n"
            f'WebUI\\Password_PBKDF2="@ByteArray({auth})"\nConnection\\UPnP=false\nConnection\\ResolvePeerCountries=false\n',
            encoding="utf-8")

    def request(self, path, form=None):
        data = urlencode(form).encode() if form is not None else None
        req = Request(self.url + "/api/v2/" + path, data=data, headers={"Referer": self.url + "/"})
        with self.opener.open(req, timeout=5) as response:
            return response.read()

    def info(self, infohash):
        rows = json.loads(self.request("torrents/info?" + urlencode({"hashes": infohash})))
        if len(rows) > 1:
            raise RuntimeError("Duplicate qBittorrent media tasks")
        return rows[0] if rows else {}

    def select(self, infohash):
        route = "torrents/files?" + urlencode({"hash": infohash})
        files = wait_for("qBittorrent file list", lambda: json.loads(self.request(route)), lambda rows: len(rows) == 3)
        if [Path(row["name"]).name for row in files] != NAMES:
            raise RuntimeError("Unexpected qBittorrent file list")
        self.request("torrents/filePrio", {"hash": infohash, "id": str(files[1]["index"]), "priority": 0})
        wait_for("qBittorrent file selection", lambda: json.loads(self.request(route)),
                 lambda rows: [row["priority"] > 0 for row in rows] == [True, False, True])

    def add(self, feed_url, torrent_path="/pack.torrent"):
        if self.request("torrents/add", {"urls": feed_url + torrent_path, "savepath": str(self.downloads)}).strip() == b"Fails.":
            raise RuntimeError("qBittorrent rejected the local torrent")

    def ensure_peer_listener(self):
        last_id = -1
        failures = []

        def bind_event():
            nonlocal last_id
            rows = json.loads(self.request(f"log/main?last_known_id={last_id}"))
            if rows:
                last_id = rows[-1]["id"]
            for row in reversed(rows):
                # qB 日志正文会翻译，监听回执中的协议、端口和类型在各语言中一致。
                port = re.search(r"\bTCP/(\d+)\b", row["message"])
                if row["type"] in (2, 8) and port and int(port[1]) == self.peer_port and HOST in row["message"]:
                    return row
            return None

        for attempt in range(3):
            event = wait_for(f"qBittorrent's TCP bind result for {HOST}:{self.peer_port}", bind_event, bool, timeout=8)
            if event["type"] == 2:  # INFO，进程已成功监听。
                return
            failures.append(event["message"])
            if attempt < 2:  # CRITICAL，进程明确报告绑定失败。
                port = free_port()
                self.request("app/setPreferences", {"json": json.dumps({"listen_port": port})})
                self.peer_port = port
        raise RuntimeError("qBittorrent could not bind its peer listener: " + " | ".join(failures))

    @contextmanager
    def running(self):
        args = [self.program, "--profile=" + str(self.profile),
                f"--webui-port={self.web_port}", f"--torrenting-port={self.peer_port}"]
        proc = subprocess.Popen(args, cwd=self.profile, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            def login():
                if proc.poll() is not None:
                    raise RuntimeError(f"qBittorrent exited during startup with code {proc.returncode}")
                try:
                    self.request("auth/login", {"username": "admin", "password": self.password})
                    return bool(self.request("app/version").strip())
                except URLError:
                    return False
            wait_for("private qBittorrent Web UI", login, bool, timeout=45)
            self.ensure_peer_listener()
            # 四个私有种子必须能同时上传，避免默认三个上传名额把新一集排在队尾。
            self.request("app/setPreferences", {"json": json.dumps({"queueing_enabled": False})})
            yield self
        finally:
            if proc.poll() is None:
                try:
                    self.request("app/shutdown", {})
                except OSError:
                    pass
                try:
                    proc.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait()

    def download_from(self, source, infohash, expected=None, *, save_path=None):
        # 私有种子没有 tracker 和 Web Seed，唯一的种源由原生 Web API 显式添加。
        def transfer():
            task = self.info(infohash)
            if task.get("progress") != 1:
                self.request("torrents/addPeers", {"hashes": infohash, "peers": f"{HOST}:{source.peer_port}"})
            return task
        wait_for("peer to transfer selected media", transfer, lambda task: task.get("progress") == 1)
        if expected is None:
            expected = {Path(TITLE) / NAMES[i]: PAYLOADS[i] for i in (0, 2)}
        for name, data in expected.items():
            if ((save_path or self.downloads) / name).read_bytes() != data:
                raise RuntimeError("qBittorrent received incorrect media bytes")


def verify_movie(service, home, central, seed):
    title = "Harbor Lights"
    with movie_feed(title) as (url, publish):
        sub = service.api("subscriptions", "POST", {
            "title_cn": title, "mikan_id": "43", "rss_url": url + "/RSS/Bangumi?bangumiId=43",
            "custom_path": str(central.downloads),
        }, status=201)
        route = f"subscriptions/{sub['id']}"
        service.api(route, "PUT", {"anime_type": "Movie", "total_episodes": 1})

        def release(suffix, status):
            name = title + suffix
            filename = name + ".mkv"
            payload = hashlib.shake_256(name.encode()).digest(54_611)
            (seed.downloads / filename).write_bytes(payload)
            infohash, torrent_path = publish(name, payload)
            seed.add(url, torrent_path)
            wait_for("qBittorrent seed to verify " + filename, lambda: seed.info(infohash), lambda row: row.get("progress") == 1)
            wait_for("movie RSS task " + filename, lambda: central.info(infohash), bool)
            central.download_from(seed, infohash, {filename: payload})

            def settled(detail):
                return any(ep["torrent_hash"] == infohash and ep["status"] in ("organized", "needs_review")
                           for ep in detail["episodes"])

            detail = wait_for("movie organization decision for " + filename, lambda: service.api(route), settled)
            matches = [ep for ep in detail["episodes"] if ep["torrent_hash"] == infohash]
            if len(matches) != 1 or matches[0]["number"] != 0 or matches[0]["status"] != status:
                raise RuntimeError(f"Expected one unnumbered {status} row for {filename}, got "
                                   f"{[(ep['number'], ep['status']) for ep in matches]}")
            return detail, matches[0], filename, payload

        detail, trailer, _, _ = release(" - Trailer", "needs_review")
        sub = detail["subscription"]
        if (len(detail["episodes"]) != 1 or trailer["final_path"] or trailer["review_kind"] != "movie_selection"
                or not trailer["can_confirm_movie"] or (sub["current_episodes"], sub["total_episodes"], sub["completed"]) != (0, 1, False)):
            raise RuntimeError("Trailer must await feature confirmation and leave movie progress at 0/1")

        print("Movie trailer awaits review with collection progress at 0/1.", flush=True)
        _, main, filename, payload = release(" [Movie]", "organized")
        # 整理记录先提交，随后更新订阅完成标记，两次 HTTP 读取可以跨越这个边界。
        detail = wait_for("movie collection to reach 1/1 and remain enabled", lambda: service.api(route),
                          lambda data: data["subscription"]["current_episodes"] == data["subscription"]["total_episodes"] == 1
                          and data["subscription"]["completed"] and data["subscription"]["enabled"])
        trailer_after = next(ep for ep in detail["episodes"] if ep["id"] == trailer["id"])
        if len(detail["episodes"]) != 2 or trailer_after["status"] != "needs_review" or trailer_after["final_path"]:
            raise RuntimeError("Organizing the feature changed the trailer's review state")
        target = Path(main["final_path"])
        library = home / "电影"
        if ([file.resolve() for file in library.rglob("*.mkv")] != [target.resolve()] or target.is_symlink()
                or not target.samefile(central.downloads / filename) or target.read_bytes() != payload):
            raise RuntimeError("Movie library must contain only the hard-linked feature from qBittorrent")
        print("Movie RSS: the standalone trailer remained in review at 0/1; the unnumbered feature was organized at 1/1.", flush=True)


def verify_new_episode(service, route, restored_home, central, seed, receiver, feed, publish, history):
    downloads = restored_home / "迁移后 Downloads"
    downloads.mkdir()
    service.api(route, "PUT", {"custom_path": str(downloads)})
    filename, payload = NAMES[1], PAYLOADS[1]
    release = Path(filename).stem

    with seed.running():
        # 首轮已验证大包的 E01 和 E03。另发独立种子补入相同订阅缺少的 E02。
        (seed.downloads / filename).write_bytes(payload)
        new_hash, torrent_path = publish(release, payload)
        seed.add(feed, torrent_path)
        wait_for("seed to verify E02", lambda: seed.info(new_hash), lambda row: row.get("progress") == 1)
        task = wait_for("restored subscription to submit E02", lambda: central.info(new_hash), bool)
        if not Path(task["save_path"]).samefile(downloads):
            raise RuntimeError("The restored subscription did not send its new path to qBittorrent")
        central.download_from(seed, new_hash, {filename: payload}, save_path=downloads)

    uploaded_before = central.info(new_hash)["uploaded_session"]
    receiver.add(feed, torrent_path)
    wait_for("new E02 receiver", lambda: receiver.info(new_hash), bool)
    receiver.download_from(central, new_hash, {filename: payload})
    uploaded = wait_for("confirmed E02 upload", lambda: central.info(new_hash)["uploaded_session"] - uploaded_before,
                        lambda count: count >= len(payload))

    detail = wait_for("restored subscription to organize E02", lambda: service.api(route),
                      lambda data: data["subscription"]["current_episodes"] == 3)
    rows = sorted(detail["episodes"], key=lambda row: row["number"])
    if [row["number"] for row in rows] != [1, 2, 3] or any(row["status"] != "organized" for row in rows):
        raise RuntimeError("The restored subscription did not retain exactly three organized episodes")
    for row in (rows[0], rows[2]):
        original = central.downloads / TITLE / NAMES[int(row["number"]) - 1]
        if row["final_path"] != history.get(row["id"]) or not Path(row["final_path"]).samefile(original):
            raise RuntimeError("Changing the download directory moved or replaced an earlier episode")
    target = Path(rows[1]["final_path"]).resolve()
    if (rows[1]["torrent_hash"].lower() != new_hash or not target.is_relative_to((restored_home / "剧集").resolve())
            or not target.samefile(downloads / filename) or target.read_bytes() != payload):
        raise RuntimeError("E02 was not hard-linked from the new download directory into the restored library")
    print(f"Post-restore qBittorrent: source_jwt=401 target_jwt=200 old_final_paths=2/2 "
          f"new_download_directory=verified episode=2 uploaded_bytes={uploaded}", flush=True)


def reconnect_and_restore_file(service, route, home, central, restarted_client):
    before = service.api(route)
    episode, = [row for row in before["episodes"] if row["number"] == 1]
    db_uri = (home / "订阅 数据.sqlite").as_uri() + "?mode=ro"

    def stored_episodes():
        with closing(sqlite3.connect(db_uri, uri=True)) as db:
            return db.execute("SELECT * FROM episodes WHERE subscription_id = ? ORDER BY id",
                              (episode["subscription_id"],)).fetchall()

    history = stored_episodes()
    target = Path(episode["final_path"])
    source = central.downloads / TITLE / NAMES[0]
    original = source.stat()
    if (target.is_symlink() or not target.resolve().is_relative_to((home / "剧集").resolve())
            or not target.samefile(source)):
        raise RuntimeError("Expected E01 to be a hard link inside the temporary library")
    # 此时 qB 已关闭。仅移除库链接，避免 Windows 下载器尚未释放文件句柄。
    target.unlink()
    digest = hashlib.sha256(source.read_bytes()).hexdigest()
    if target.exists() or digest != hashlib.sha256(PAYLOADS[0]).hexdigest():
        raise RuntimeError("Removing the temporary E01 library link did not preserve its download")

    restarted_client.enter_context(central.running())
    if not service.api("downloader/test", "POST", {})["success"]:
        raise RuntimeError("The source Ani-Go instance did not reconnect to qBittorrent")
    wait_for("qBittorrent to resume the complete pack", lambda: central.info(episode["torrent_hash"]),
             lambda task: task.get("progress") == 1 and not task["state"].startswith("checking"))
    started = time.monotonic()
    check = service.api(route + "/verify-files", "POST", {"ids": [episode["id"]]})
    if check["volume_guarded"] or [(row["id"], row["status"]) for row in check["results"]] != [(episode["id"], "missing")]:
        raise RuntimeError("Ani-Go did not find exactly the removed E01 library link missing")
    response = service.api(f"{route}/episodes/{episode['id']}/restore-file", "POST", {"confirm_unmarked_volume": True})
    if (response != {"restored": True} or target.is_symlink() or not target.samefile(source)
            or not os.path.samestat(target.stat(), original) or hashlib.sha256(target.read_bytes()).hexdigest() != digest):
        raise RuntimeError("The restore did not recreate the E01 hard link to the original download")
    after = service.api(route)
    progress = ("current_episodes", "total_episodes", "completed")
    for detail in (before, after):
        if tuple(detail["subscription"][field] for field in progress) != (2, 3, False):
            raise RuntimeError("Restoring E01 changed the subscription progress")
    if before["episodes"] != after["episodes"] or history != stored_episodes():
        raise RuntimeError("Restoring E01 changed the original episode history")
    print(f"Library file restore: episode=1 hard_link=verified sha256={digest} history=unchanged progress=2/3 "
          f"elapsed_ms={round((time.monotonic() - started) * 1000)}", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--qbittorrent", default="qbittorrent-nox", help="qBittorrent GUI or nox executable")
    args = parser.parse_args()
    program = shutil.which(args.qbittorrent)
    if not program:
        parser.error("qBittorrent executable not found")
    with (tempfile.TemporaryDirectory(prefix="Ani-Go qB ") as folder, web_seed(http_seed=False) as (url, infohash),
          ExitStack() as restarted_client):
        root = Path(folder)
        home = root / "app"
        central = QB(program, root / "primary", root / "downloads")
        seed = QB(program, root / "seed", root / "seed media")
        receiver = QB(program, root / "receiver", root / "received media")
        for qb in (central, seed, receiver):
            qb.downloads.mkdir()
        original = seed.downloads / TITLE
        original.mkdir()
        for name, data in zip(NAMES, PAYLOADS):
            (original / name).write_bytes(data)
        env = dict(DEFAULT_DOWNLOADER="qbittorrent", QB_HOST=central.url, QB_USER="admin", QB_PASS=central.password,
                   RSS_INTERVAL="1s", ORGANIZER_INTERVAL="1s", MIKAN_RSS_MODE="personal", MIKAN_DOMAIN=url,
                   MIKAN_MIRROR_DOMAINS=url, MIKAN_PROXY_DOMAIN=url, MIKAN_RSS_URL=url+"/RSS/Bangumi?bangumiId=42",
                   DISK_MIN_FREE_GB="0", USE_HARDLINK="true", SEED_CLEANUP_ENABLED="false")
        command, backups, port = [str(args.binary.resolve(strict=True))], root / "backups", free_port()
        offline = "http://127.0.0.1:9"
        with start(command, home, backups, port, secrets.token_urlsafe(24), offline, env_overrides=env) as service:
            with central.running():
                version = central.request("app/version").decode().strip()
                print(f"Connected to qBittorrent {version} on {sys.platform}", flush=True)
                route = create_pack_subscription(service, url, central.downloads)
                wait_for("RSS submitted to qBittorrent", lambda: central.info(infohash), bool)
                central.select(infohash)
                with seed.running():
                    seed.add(url)
                    wait_for("source hashes verified", lambda: seed.info(infohash), lambda row: row.get("progress") == 1)
                    central.download_from(seed, infohash)
                    verify_movie(service, home, central, seed)
                verify_library(service, route, home, central.downloads)
                for row in service.api(route)["episodes"]:
                    source = central.downloads / TITLE / NAMES[int(row["number"]) - 1]
                    if not source.samefile(row["final_path"]):
                        raise RuntimeError("Selected download and library media should share a hard link")
                print("Selected S01E01 and S01E03 arrived from the seed and were hard-linked into the library.", flush=True)
            if service.api("downloader/test", "POST", {})["success"]:
                raise RuntimeError("Ani-Go reported a stopped qBittorrent client online")
            reconnect_and_restore_file(service, route, home, central, restarted_client)
            history = {row["id"]: row["final_path"] for row in service.api(route)["episodes"]}
            service.api("backup/create", "POST", {"include_episodes": True})
            archive, = service.api("backup/list")
        restored_home = root / "恢复 实例"
        with movie_feed(TITLE) as (feed, publish):
            # 个人 RSS 给已恢复的订阅补入一个新发行，原合集的独立 RSS 仍可轮询。
            env["MIKAN_RSS_URL"] = feed + "/RSS/MyBangumi"
            with (start(command, restored_home, backups, port, secrets.token_urlsafe(24), offline, env_overrides=env) as restored,
                  receiver.running()):
                if restored.api("subscriptions"):
                    raise RuntimeError("The restored instance must start with an empty database")
                restored.api("backup/restore", "POST", {"name": archive["name"]})
                service.api("me", status=401)
                if restored.api("me").get("username") != "admin":
                    raise RuntimeError("Restoring the archive changed the target session")
                if not restored.api("downloader/test", "POST", {})["success"]:
                    raise RuntimeError("Ani-Go did not reconnect to qBittorrent")
                wait_for("completed torrent restored after restart", lambda: central.info(infohash), lambda row: row.get("progress") == 1)
                receiver.add(url)
                wait_for("fresh receiver torrent", lambda: receiver.info(infohash), bool)
                receiver.select(infohash)
                receiver.download_from(central, infohash)
                uploaded = wait_for("confirmed qBittorrent upload", lambda: central.info(infohash)["uploaded_session"],
                                    lambda count: count >= len(PAYLOADS[0]) + len(PAYLOADS[2]))
                verify_new_episode(restored, route, restored_home, central, seed, receiver, feed, publish, history)
                queue = restored.api("downloads")
                if sum(row["hash"].lower() == infohash for row in queue) != 1:
                    raise RuntimeError("Duplicate or missing qBittorrent task in Ani-Go")
        print(f"Passed: qBittorrent {version}; selected S01E01 and S01E03, restarted, uploaded {uploaded} bytes to a fresh peer.")


if __name__ == "__main__":
    main()
