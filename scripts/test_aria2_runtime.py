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
from threading import Event

from test_platform_runtime import HOST, Service, start
from torrent_fixture import create_pack_subscription, free_port, verify_library, wait_for, web_seed


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
def real_client(binary, root):
    client = Aria2(free_port(), secrets.token_urlsafe(24))
    config = root / "aria.conf"
    session = root / "aria.session"
    session.touch()
    config.write_text("\n".join([
        "enable-rpc=true", "rpc-listen-all=false", f"rpc-listen-port={client.port}", f"rpc-secret={client.secret}",
        "interface=" + HOST, "disable-ipv6=true", "enable-dht=false", "enable-dht6=false",
        "enable-peer-exchange=false", "bt-enable-lpd=false", "seed-time=0", f"listen-port={free_port()}",
        "file-allocation=none", "no-proxy=" + HOST, f"save-session={session}", f"input-file={session}",
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

    def pieces(job):
        return int(job.get("bitfield") or "0", 16)

    partial = wait_for("verified pieces before restart", lambda: aria.rpc("tellStatus", gid),
                       lambda job: pieces(job) and int(job["completedLength"]) < int(job["totalLength"]))
    media_ready.clear()
    restart(service)

    # 还原会话时 aria2 可以分配新的子任务 GID，稳定标识是种子哈希和元数据任务。
    def media_jobs():
        tasks = aria.rpc("tellActive") + aria.rpc("tellWaiting", 0, 50) + aria.rpc("tellStopped", 0, 50)
        unique = {job["gid"]: job for job in tasks if job.get("infoHash") == infohash}
        return list(unique.values())

    # 禁止 Web Seed 返回新的媒体字节，直到本次任务已从本地控制文件读回原有分片。
    saved_bytes = int(partial["completedLength"])
    saved_pieces = pieces(partial)

    def recovered(jobs):
        return (jobs and int(jobs[0]["completedLength"]) >= saved_bytes
                and pieces(jobs[0]) & saved_pieces == saved_pieces)

    restored = wait_for("saved progress with the Web Seed blocked", media_jobs, recovered, timeout=25)
    if len(restored) != 1 or restored[0].get("following") != children[0]["following"]:
        raise RuntimeError("aria2 did not restore a unique torrent from the original submission")
    gid = restored[0]["gid"]
    if [file["selected"] for file in aria.rpc("getFiles", gid)] != WANTED:
        raise RuntimeError("aria2 lost the file selection across restart")
    print(f"Restored {restored[0]['completedLength']} bytes (before shutdown: {saved_bytes}) and the original selection.")
    media_ready.set()

    def status():
        job = aria.rpc("tellStatus", gid, ["status", "errorCode"])
        if job["status"] == "error":
            raise RuntimeError(f"aria2 download failed with code {job.get('errorCode')}")
        return job["status"]

    wait_for("Web Seed download", status, lambda state: state == "complete")
    verify_library(service, route, home, downloads)
    if len(media_jobs()) != 1:
        raise RuntimeError("Duplicate aria2 media tasks after completion")
    queue = service.api("downloads")
    if sum(job["hash"].lower() in (gid, infohash) for job in queue) != 1:
        raise RuntimeError("Unexpected number of Ani-Go media tasks after reconnect")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path, help="Built Ani-Go executable")
    parser.add_argument("--aria2c", default="aria2c", help="aria2c executable name or path")
    args = parser.parse_args()
    aria_path = shutil.which(args.aria2c)
    if not aria_path:
        parser.error("aria2c executable not found")
    with tempfile.TemporaryDirectory(prefix="Ani-Go aria2 ") as folder, socket.socket() as blocked:
        root = Path(folder)
        home = root / "app"
        downloads = root / "downloads"
        downloads.mkdir()
        blocked.bind((HOST, 0))
        offline = f"http://{HOST}:{blocked.getsockname()[1]}"
        media_ready = Event()
        with web_seed(media_ready) as (url, infohash), real_client(str(Path(aria_path).resolve()), root) as (aria, version, restart):
            overrides = dict(DEFAULT_DOWNLOADER="aria2", ARIA2_HOST=f"http://{HOST}:{aria.port}", ARIA2_SECRET=aria.secret,
                             RSS_INTERVAL="1s", ORGANIZER_INTERVAL="1s", MIKAN_RSS_MODE="personal", MIKAN_DOMAIN=url,
                             MIKAN_MIRROR_DOMAINS=url, MIKAN_PROXY_DOMAIN=url, MIKAN_RSS_URL=url + "/RSS/Bangumi?bangumiId=42",
                             DISK_MIN_FREE_GB="0", USE_HARDLINK="true")
            with start([str(args.binary.resolve(strict=True))], home, root / "backups", free_port(),
                       secrets.token_urlsafe(24), offline, env_overrides=overrides) as service:
                verify(service, aria, restart, media_ready, url, infohash, home, downloads)
    print(f"Passed: aria2 {version}, RSS, file selection, interrupted download and session recovery, Web Seed and library content.")


if __name__ == "__main__":
    main()
