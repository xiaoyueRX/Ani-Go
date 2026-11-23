"""用回环 HTTP Web Seed 验收 Ani-Go 从 RSS 投递到真实 aria2 的选集和入库。"""

import argparse
from contextlib import contextmanager
from http.client import HTTPException
import json
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
from threading import Condition, Event

from test_platform_runtime import HOST, Service, start
from test_portable_backup import import_backup, load_backup, verify_history
from torrent_fixture import NAMES, PAYLOADS, PIECE_SIZE, TITLE, create_pack_subscription, free_port, movie_feed, verify_library, wait_for, web_seed


WANTED = ["true", "false", "true"]


class Aria2(Service):
    def __init__(self, port, secret):
        super().__init__(port)
        self.secret = secret

    def rpc(self, name, *params):
        _, body = self.request("/jsonrpc", "POST", {
            "jsonrpc": "2.0", "id": 1, "method": "aria2." + name, "params": ["token:" + self.secret, *params],
        })
        reply = json.loads(body)
        if "error" in reply:
            raise RuntimeError(f"aria2.{name} returned error {reply['error']['code']}")
        return reply["result"]


@contextmanager
def real_client(binary, root, default_dir=None):
    client = Aria2(free_port(), secrets.token_urlsafe(24))
    config = root / "aria.conf"
    session = root / "aria.session"
    session.touch()
    config.write_text("\n".join([
        "enable-rpc=true", "rpc-listen-all=false", f"rpc-listen-port={client.port}", f"rpc-secret={client.secret}",
        "interface=" + HOST, "disable-ipv6=true", "enable-dht=false", "enable-dht6=false",
        "enable-peer-exchange=false", "bt-enable-lpd=false", "seed-time=0", f"listen-port={free_port()}",
        f"dir={default_dir or root}", "file-allocation=none", "no-proxy=" + HOST, f"save-session={session}", f"input-file={session}",
        "save-session-interval=1", "auto-save-interval=1",
    ]), encoding="utf-8")

    def launch(*options):
        return subprocess.Popen([binary, "--conf-path=" + str(config), *options], cwd=root,
                                stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def version():
        if proc.poll() is not None:
            raise RuntimeError(f"aria2c exited with status {proc.returncode}")
        try:
            return client.rpc("getVersion")
        except (OSError, HTTPException):
            return None

    def rpc_closed():
        with socket.socket() as probe:
            probe.settimeout(1)
            return probe.connect_ex((HOST, client.port)) != 0

    def stop():
        # Windows 包管理器有时启动包装进程，由真正持有工作目录的 aria2 自身退出。
        try:
            if not rpc_closed():
                client.rpc("shutdown")
            proc.wait(timeout=20)
        finally:
            if proc.poll() is None:
                proc.kill()
                proc.wait()
        wait_for("aria2 RPC listener to close", rpc_closed, bool, timeout=10)

    def restart(service):
        nonlocal proc
        client.rpc("saveSession")
        stop()
        if service.api("downloader/test", "POST", {})["success"]:
            raise RuntimeError("Ani-Go reported aria2 online after its process exited")
        proc = launch("--max-overall-download-limit=48K")
        wait_for("restarted aria2 RPC", version, bool)
        if not service.api("downloader/test", "POST", {})["success"]:
            raise RuntimeError("Ani-Go did not reconnect to the restarted aria2")

    # 限速为轮询真实的中途进度留出时间。
    proc = launch("--max-overall-download-limit=16K")
    try:
        details = wait_for("aria2 RPC", version, bool)
        yield client, details["version"], restart
    finally:
        stop()


def media_jobs(aria, infohash):
    tasks = aria.rpc("tellActive") + aria.rpc("tellWaiting", 0, 50) + aria.rpc("tellStopped", 0, 50)
    return list({task["gid"]: task for task in tasks if task.get("infoHash") == infohash}.values())


def partially_downloaded(job):
    return (int(job.get("bitfield") or "0", 16) != 0
            and 0 < int(job["completedLength"]) < int(job["totalLength"]))


def restart_download(service, aria, restart, infohash, partial):
    saved_bytes = int(partial["completedLength"])
    saved_pieces = int(partial["bitfield"], 16)
    restart(service)

    # 还原会话时子任务 GID 可变，按种子哈希查找并核对元数据父任务及已完成的分片。
    def recovered(jobs):
        return (len(jobs) == 1 and int(jobs[0]["completedLength"]) >= saved_bytes
                and int(jobs[0].get("bitfield") or "0", 16) & saved_pieces == saved_pieces)

    job, = wait_for("saved progress with the Web Seed blocked", lambda: media_jobs(aria, infohash), recovered, timeout=25)
    if not partial.get("following") or job.get("following") != partial["following"]:
        raise RuntimeError("aria2 did not resume the original submission")
    print(f"Restored {job['completedLength']} bytes (before shutdown: {saved_bytes}).")
    return job


def verify(service, aria, restart, media_ready, url, infohash, home, downloads):
    route = create_pack_subscription(service, url, downloads)

    def active_children():
        return [job for job in aria.rpc("tellActive") if job.get("infoHash") == infohash and job.get("files")]

    children = wait_for("the RSS torrent in aria2", active_children, bool)
    if len(children) != 1:
        raise RuntimeError("Expected a single torrent submitted by Ani-Go")
    gid = children[0]["gid"]
    aria.rpc("forcePause", gid)
    wait_for("paused torrent for file selection", lambda: aria.rpc("tellStatus", gid),
             lambda job: job["status"] == "paused")
    aria.rpc("changeOption", gid, {"select-file": "1,3"})
    if [file["selected"] for file in aria.rpc("getFiles", gid)] != WANTED:
        raise RuntimeError("aria2 did not select only episodes 1 and 3")
    aria.rpc("unpause", gid)
    media_ready.set()

    partial = wait_for("verified pieces before restart", lambda: aria.rpc("tellStatus", gid), partially_downloaded)
    media_ready.clear()
    resumed = restart_download(service, aria, restart, infohash, partial)
    gid = resumed["gid"]
    if [file["selected"] for file in aria.rpc("getFiles", gid)] != WANTED:
        raise RuntimeError("aria2 lost the file selection across restart")
    media_ready.set()

    def status():
        job = aria.rpc("tellStatus", gid, ["status", "errorCode"])
        if job["status"] == "error":
            raise RuntimeError(f"aria2 download failed with code {job.get('errorCode')}")
        return job["status"]

    wait_for("Web Seed download", status, lambda state: state == "complete")
    verify_library(service, route, home, downloads)
    if len(media_jobs(aria, infohash)) != 1:
        raise RuntimeError("Duplicate aria2 media tasks after completion")
    queue = service.api("downloads")
    if sum(job["hash"].lower() in (gid, infohash) for job in queue) != 1:
        raise RuntimeError("Unexpected number of Ani-Go media tasks after reconnect")


class PartialWebSeed:
    """限制单文件 Web Seed 可发送的字节区间，重启期间将区间缩为零。"""

    def __init__(self, limit):
        self.limit = limit
        self.sent = 0
        self.changed = Condition()

    def set_limit(self, limit):
        with self.changed:
            self.limit = limit
            self.changed.notify_all()
            return self.sent

    def write(self, stream, data, start, stop):
        with self.changed:
            while start < stop:
                if not self.changed.wait_for(lambda: self.limit > start, timeout=90):
                    return
                end = min(stop, self.limit)
                stream.write(data[start:end])
                self.sent += end - start
                start = end


def app_environment(aria, url, rss_path):
    return dict(DEFAULT_DOWNLOADER="aria2", ARIA2_HOST=f"http://{HOST}:{aria.port}", ARIA2_SECRET=aria.secret,
                RSS_INTERVAL="1s", ORGANIZER_INTERVAL="1s", MIKAN_RSS_MODE="personal", MIKAN_DOMAIN=url,
                MIKAN_MIRROR_DOMAINS=url, MIKAN_PROXY_DOMAIN=url, MIKAN_RSS_URL=url + rss_path,
                DISK_MIN_FREE_GB="0", USE_HARDLINK="true")


def verify_restore(binary, aria_path, root, offline, archive):
    home, backups = root / "app", root / "backups"
    library, downloads, default = home / "剧集", root / "更新后 Downloads", root / "aria-default"
    downloads.mkdir()
    default.mkdir()
    port, password = free_port(), secrets.token_urlsafe(32)
    payload = PAYLOADS[1]
    seed = PartialWebSeed(4 * PIECE_SIZE)

    def app():
        return start([binary], home, backups, port, password, offline, env_overrides=env)

    def transfer(service, route, number):
        filename = NAMES[number - 1]
        infohash, _ = publish(Path(filename).stem, payload)

        def partial(jobs):
            return len(jobs) == 1 and partially_downloaded(jobs[0])

        job, = wait_for("verified E02 pieces", lambda: media_jobs(aria, infohash), partial)
        if not Path(job["dir"]).samefile(downloads) or not Path(aria.rpc("getGlobalOption")["dir"]).samefile(default):
            raise RuntimeError("aria2 did not receive the restored subscription's new directory")
        sent = seed.set_limit(0)
        resumed = restart_download(service, aria, restart, infohash, job)
        bytes_while_blocked = seed.set_limit(len(payload)) - sent
        if (bytes_while_blocked != 0 or not partially_downloaded(resumed)
                or not Path(resumed["dir"]).samefile(downloads)
                or not Path(aria.rpc("getGlobalOption")["dir"]).samefile(default)):
            raise RuntimeError("aria2 did not recover the partial E02 in its original directory with the Web Seed blocked")
        print(f"Web Seed sent {bytes_while_blocked} bytes during restart; recovered E02 bitfield={resumed['bitfield']}.")
        file, = resumed["files"]
        original = downloads / filename
        if file["selected"] != "true" or not Path(file["path"]).samefile(original):
            raise RuntimeError("The restored aria2 session selected a different E02 file")
        done, = wait_for("aria2 E02 completion", lambda: media_jobs(aria, infohash),
                         lambda jobs: len(jobs) == 1 and jobs[0]["status"] == "complete")
        detail = wait_for("organized E02", lambda: service.api(route),
                          lambda d: any(ep["torrent_hash"] == infohash and ep["status"] == "organized" for ep in d["episodes"]))
        episode, = [ep for ep in detail["episodes"] if ep["number"] == number]
        final = Path(episode["final_path"])
        if (episode["torrent_hash"] != infohash or int(done["completedLength"]) != len(payload)
                or episode["file_size"] != len(payload) or original.is_symlink() or final.is_symlink()
                or not final.resolve().is_relative_to(library.resolve()) or not final.samefile(original)
                or original.read_bytes() != payload or any(default.iterdir())):
            raise RuntimeError("The E02 download and hard-linked library file differ from the reference")
        return episode

    with movie_feed(TITLE, http_seed=True, write_media=seed.write) as (url, publish):
        with real_client(aria_path, root, default_dir=default) as (aria, version, restart):
            env = app_environment(aria, url, "/RSS/MyBangumi")
            try:
                import_backup(app, transfer, archive, backups, library, downloads)
            finally:
                seed.set_limit(len(payload))
    # 来源和下载器均已退出，独立重启应用以核实两条历史和库中的实际字节。
    with app() as service:
        route = verify_history(service, archive[2]["ID"], archive[2], archive[3], downloads, 2)
        episode, = [ep for ep in service.api(route)["episodes"] if ep["number"] == 2]
        final = Path(episode["final_path"])
        if episode["status"] != "organized" or not final.samefile(downloads / NAMES[1]) or final.read_bytes() != payload:
            raise RuntimeError("Restarting Ani-Go did not preserve the newly organized E02")
    print(f"Passed: Windows archive, aria2 {version}, partial E02 recovered with its Web Seed blocked, {len(payload)} final bytes verified.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path, help="Built Ani-Go executable")
    parser.add_argument("--aria2c", default="aria2c", help="aria2c executable name or path")
    parser.add_argument("--restore-windows-backup", type=Path, metavar="DIR",
                        help="Restore a native Windows full JSON and resume E02 in a new directory")
    args = parser.parse_args()
    aria_path = shutil.which(args.aria2c)
    if not aria_path:
        parser.error("aria2c executable not found")
    with tempfile.TemporaryDirectory(prefix="Ani-Go aria2 ") as folder, socket.socket() as blocked:
        root = Path(folder)
        blocked.bind((HOST, 0))
        offline = f"http://{HOST}:{blocked.getsockname()[1]}"
        binary = str(args.binary.resolve(strict=True))
        aria_path = str(Path(aria_path).resolve())
        if args.restore_windows_backup:
            archive = load_backup(args.restore_windows_backup, required_source="Windows")
            verify_restore(binary, aria_path, root, offline, archive)
            return
        home, downloads = root / "app", root / "downloads"
        downloads.mkdir()
        media_ready = Event()
        with web_seed(media_ready) as (url, infohash), real_client(aria_path, root) as (aria, version, restart):
            overrides = app_environment(aria, url, "/RSS/Bangumi?bangumiId=42")
            with start([binary], home, root / "backups", free_port(), secrets.token_urlsafe(24), offline,
                       env_overrides=overrides) as service:
                verify(service, aria, restart, media_ready, url, infohash, home, downloads)
    print(f"Passed: aria2 {version}, RSS, file selection, interrupted download and session recovery, Web Seed and library content.")


if __name__ == "__main__":
    main()
