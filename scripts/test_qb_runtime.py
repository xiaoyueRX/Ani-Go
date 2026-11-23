"""用真实的 qBittorrent 验收 RSS、选集、入库及重启后的独立做种。"""

import argparse
import base64
from contextlib import contextmanager
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys
import tempfile
from urllib.error import URLError
from urllib.parse import urlencode
from urllib.request import build_opener, HTTPCookieProcessor, ProxyHandler, Request

from test_platform_runtime import HOST, start
from torrent_fixture import create_pack_subscription, free_port, NAMES, PAYLOADS, TITLE, verify_library, wait_for, web_seed


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

    def add(self, feed_url):
        if self.request("torrents/add", {"urls": feed_url + "/pack.torrent", "savepath": str(self.downloads)}).strip() != b"Ok.":
            raise RuntimeError("qBittorrent rejected the local torrent")

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
                    return self.request("auth/login", {"username": "admin", "password": self.password}) == b"Ok."
                except URLError:
                    return False
            wait_for("private qBittorrent Web UI", login, bool, timeout=45)
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

    def download_from(self, source, infohash):
        # 私有种子没有 tracker 和 Web Seed，唯一的种源由原生 Web API 显式添加。
        def transfer():
            task = self.info(infohash)
            if task.get("progress") != 1:
                self.request("torrents/addPeers", {"hashes": infohash, "peers": f"{HOST}:{source.peer_port}"})
            return task
        wait_for("peer to transfer selected media", transfer, lambda task: task.get("progress") == 1)
        for i in (0, 2):
            if (self.downloads / TITLE / NAMES[i]).read_bytes() != PAYLOADS[i]:
                raise RuntimeError("qBittorrent received incorrect media bytes")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--qbittorrent", default="qbittorrent-nox", help="qBittorrent GUI or nox executable")
    args = parser.parse_args()
    program = shutil.which(args.qbittorrent)
    if not program:
        parser.error("qBittorrent executable not found")
    with tempfile.TemporaryDirectory(prefix="Ani-Go qB ") as folder, web_seed(http_seed=False) as (url, infohash):
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
        with start([str(args.binary.resolve(strict=True))], home, root/"backups", free_port(),
                   secrets.token_urlsafe(24), "http://127.0.0.1:9", env_overrides=env) as service:
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
                verify_library(service, route, home, central.downloads)
                for row in service.api(route)["episodes"]:
                    source = central.downloads / TITLE / NAMES[int(row["number"]) - 1]
                    if not source.samefile(row["final_path"]):
                        raise RuntimeError("Selected download and library media should share a hard link")
                print("Selected S01E01 and S01E03 arrived from the seed and were hard-linked into the library.", flush=True)
            if service.api("downloader/test", "POST", {})["success"]:
                raise RuntimeError("Ani-Go reported a stopped qBittorrent client online")
            with central.running(), receiver.running():
                if not service.api("downloader/test", "POST", {})["success"]:
                    raise RuntimeError("Ani-Go did not reconnect to qBittorrent")
                wait_for("completed torrent restored after restart", lambda: central.info(infohash), lambda row: row.get("progress") == 1)
                receiver.add(url)
                wait_for("fresh receiver torrent", lambda: receiver.info(infohash), bool)
                receiver.select(infohash)
                receiver.download_from(central, infohash)
                uploaded = wait_for("confirmed qBittorrent upload", lambda: central.info(infohash)["uploaded_session"],
                                    lambda count: count >= len(PAYLOADS[0]) + len(PAYLOADS[2]))
                queue = service.api("downloads")
                if sum(row["hash"].lower() == infohash for row in queue) != 1:
                    raise RuntimeError("Duplicate or missing qBittorrent task in Ani-Go")
        print(f"Passed: qBittorrent {version}; selected S01E01 and S01E03, restarted, uploaded {uploaded} bytes to a fresh peer.")


if __name__ == "__main__":
    main()
