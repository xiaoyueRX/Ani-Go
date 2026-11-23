"""用 Transmission 验收 RSS 选集、断线恢复、完整备份换目录与重启后做种。"""

import argparse
import base64
import hashlib
from contextlib import contextmanager
from http.client import HTTPConnection, HTTPException
import json
from pathlib import Path
import secrets
import shutil
import socket
import struct
import subprocess
import tempfile
from threading import Event
import time

from test_platform_runtime import HOST, start
from test_portable_backup import import_backup, load_backup, verify_history
from torrent_fixture import NAMES, PAYLOADS, PIECE_SIZE, TITLE, create_pack_subscription, free_port, movie_feed, verify_library, wait_for, web_seed


WANTED = [True, False, True]
# 4.0.5 的 HTTP Web Seed 在未选文件与下一选中文件共用分片时可能停滞。
# 各集在分片边界结束，使多个版本都能验证同一套选集、恢复和做种流程。
ALIGNED_PAYLOADS = [data[:len(data) // PIECE_SIZE * PIECE_SIZE] for data in PAYLOADS]


class Transmission:
    def __init__(self, port):
        self.port = port
        self.session_id = ""

    def rpc(self, method, **arguments):
        # 首次连接和 daemon 重启均需从 HTTP 409 取新的会话标识。
        for _ in range(2):
            conn = HTTPConnection(HOST, self.port, timeout=5)
            try:
                conn.request("POST", "/transmission/rpc", json.dumps(dict(method=method, arguments=arguments)),
                             {"Content-Type": "application/json", "X-Transmission-Session-Id": self.session_id})
                response = conn.getresponse()
                data = response.read()
                if response.status == 409:
                    self.session_id = response.getheader("X-Transmission-Session-Id", "")
                    continue
                if response.status != 200:
                    raise RuntimeError(f"Transmission {method} returned HTTP {response.status}")
                reply = json.loads(data)
                if reply["result"] != "success":
                    raise RuntimeError(f"Transmission {method}: {reply['result']}")
                return reply["arguments"]
            finally:
                conn.close()
        raise RuntimeError("Transmission rejected its session ID")

    def torrent(self, infohash):
        fields = ["hashString", "status", "leftUntilDone", "sizeWhenDone", "pieces", "files", "fileStats", "errorString", "downloadDir"]
        rows = self.rpc("torrent-get", fields=fields)["torrents"]
        if not rows:
            return None
        if len(rows) != 1 or rows[0]["hashString"] != infohash:
            raise RuntimeError("Expected one Transmission torrent with the RSS infohash")
        if rows[0]["errorString"]:
            raise RuntimeError("Transmission download failed: " + rows[0]["errorString"])
        return rows[0]


@contextmanager
def daemon(binary, config_dir, rpc):
    with subprocess.Popen([binary, "--foreground", "--config-dir", str(config_dir)], cwd=config_dir,
                          stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL) as proc:
        def session():
            if proc.poll() is not None:
                raise RuntimeError(f"Transmission exited with status {proc.returncode}")
            try:
                return rpc.rpc("session-get")
            except (OSError, HTTPException):
                return None

        try:
            yield wait_for("Transmission RPC", session, bool)["version"]
        finally:
            if proc.poll() is None:
                try:
                    # Windows 的 terminate() 不保存恢复状态，使用客户端自身的关机接口。
                    rpc.rpc("session-close")
                except (OSError, HTTPException):
                    pass
                try:
                    proc.wait(timeout=45)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait()
                    raise RuntimeError("Transmission did not shut down after session-close")
            if proc.returncode:
                raise RuntimeError(f"Transmission exited with status {proc.returncode}")


def check_wanted(record):
    if [f["wanted"] for f in record["fileStats"]] != WANTED:
        raise RuntimeError("Transmission did not retain the selected episodes 1 and 3")
    if [entry["length"] for entry in record["files"]] != [len(data) for data in ALIGNED_PAYLOADS]:
        raise RuntimeError("Transmission file sizes differ from the Web Seed content")


def verify_peer_upload(port, infohash, expected):
    # 独立回环对等端依次请求分片，同时核对片号、长度和实际媒体字节。
    def read_exact(peer, size):
        data = bytearray()
        while len(data) < size:
            chunk = peer.recv(size - len(data))
            if not chunk:
                raise RuntimeError("Transmission closed the peer connection prematurely")
            data.extend(chunk)
        return data

    protocol = b"\x13BitTorrent protocol"
    torrent_hash = bytes.fromhex(infohash)
    with socket.create_connection((HOST, port), timeout=10) as peer:
        peer.settimeout(20)
        peer.sendall(protocol + bytes(8) + torrent_hash + b"-AG0001-" + secrets.token_bytes(12))
        handshake = read_exact(peer, 68)
        if handshake[:20] != protocol or handshake[28:48] != torrent_hash:
            raise RuntimeError("Transmission returned an invalid peer handshake")
        peer.sendall(struct.pack("!IB", 1, 2))  # interested
        deadline = time.monotonic() + 45
        unchoked = False
        pending = False
        offset = 0
        received = hashlib.sha256()
        while time.monotonic() < deadline and offset < len(expected):
            piece = offset // PIECE_SIZE
            block = expected[offset:offset + PIECE_SIZE]
            if unchoked and not pending:
                peer.sendall(struct.pack("!IBIII", 13, 6, piece, 0, len(block)))
                pending = True
            length = struct.unpack("!I", read_exact(peer, 4))[0]
            if not length:
                continue
            if length > (1 << 20):
                raise RuntimeError("Unexpected peer message size")
            message = read_exact(peer, length)
            if message[0] == 0:  # choke
                unchoked = pending = False
            elif message[0] == 1:  # unchoke
                unchoked = True
            elif message[0] == 7:  # piece
                if not pending or message[1:9] != struct.pack("!II", piece, 0) or message[9:] != block:
                    raise RuntimeError(f"Transmission uploaded different content for piece {piece}")
                received.update(message[9:])
                offset += len(block)
                pending = False
        if offset != len(expected):
            raise RuntimeError(f"Transmission uploaded only {offset} of {len(expected)} requested bytes")
        print(f"Verified {offset} uploaded bytes from Transmission after library organization, sha256={received.hexdigest()}.")


def verify_completed_task(service, infohash):
    queue = service.api("downloads")
    if len(queue) != 1 or queue[0]["hash"] != infohash or not queue[0]["complete"]:
        raise RuntimeError("Ani-Go did not reconnect to the original completed torrent")


def verified_pieces(record):
    return int.from_bytes(base64.b64decode(record["pieces"]), "big")


def configure_daemon(config, rpc, downloads):
    config.mkdir()
    downloads.mkdir()
    settings = {
        "rpc-enabled": True, "rpc-port": rpc.port, "rpc-bind-address": HOST,
        "bind-address-ipv4": HOST, "bind-address-ipv6": "::1", "peer-port": free_port(),
        "pex-enabled": False, "dht-enabled": False, "lpd-enabled": False, "port-forwarding-enabled": False,
        "watch-dir-enabled": False, "download-dir": str(downloads), "incomplete-dir-enabled": False,
        "ratio-limit-enabled": False, "idle-seeding-limit-enabled": False,
        "speed-limit-down": 35, "speed-limit-down-enabled": True,
    }
    (config / "settings.json").write_text(json.dumps(settings), encoding="utf-8")
    return settings["peer-port"]


def app_environment(rpc, url, rss_path):
    return dict(DEFAULT_DOWNLOADER="transmission", TR_HOST=f"http://{HOST}:{rpc.port}",
                RSS_INTERVAL="1s", ORGANIZER_INTERVAL="1s", MIKAN_RSS_MODE="personal", MIKAN_DOMAIN=url,
                MIKAN_MIRROR_DOMAINS=url, MIKAN_PROXY_DOMAIN=url, MIKAN_RSS_URL=url + rss_path,
                DISK_MIN_FREE_GB="0", USE_HARDLINK="true", SEED_CLEANUP_ENABLED="false")


def verify(binary, transmission_bin, root, offline, url, infohash, media_ready):
    home, downloads, config = root / "app", root / "downloads", root / "transmission"
    rpc = Transmission(free_port())
    peer_port = configure_daemon(config, rpc, downloads)
    overrides = app_environment(rpc, url, "/RSS/Bangumi?bangumiId=42")
    with start([binary], home, root / "backups", free_port(), secrets.token_urlsafe(24), offline, env_overrides=overrides) as service:
        with daemon(transmission_bin, config, rpc) as version:
            route = create_pack_subscription(service, url, downloads)

            def torrent():
                return rpc.torrent(infohash)

            wait_for("RSS submission to Transmission", torrent, bool)
            rpc.rpc("torrent-set", ids=[infohash], **{"files-wanted": [0, 2], "files-unwanted": [1]})
            media_ready.set()
            partial = wait_for("partial Web Seed download", torrent,
                               lambda t: 0 < t["leftUntilDone"] < t["sizeWhenDone"] and verified_pieces(t))
            media_ready.clear()
            check_wanted(partial)
            saved_pieces = verified_pieces(partial)

        service.request("/api/downloads", status=502)
        rows = service.api(route)["episodes"]
        if len(rows) != 1 or rows[0]["status"] != "downloading":
            raise RuntimeError("The pending RSS task changed while Transmission was offline")
        with daemon(transmission_bin, config, rpc):
            def has_saved_pieces(row):
                return row and (verified_pieces(row) & saved_pieces) == saved_pieces

            restored = wait_for("persisted pieces with the Web Seed blocked", torrent, has_saved_pieces, timeout=30)
            check_wanted(restored)
            print(f"Restored the original infohash, file selection and {saved_pieces.bit_count()} verified pieces offline.")
            media_ready.set()
            try:
                seeded = wait_for("seeding after recovery", torrent, lambda t: t["status"] == 6 and t["leftUntilDone"] == 0)
            except RuntimeError as exc:
                row = torrent()
                if row:
                    raise RuntimeError(f"{exc} (Transmission {version}, status={row['status']}, "
                                       f"remaining={row['leftUntilDone']}, fileStats={row['fileStats']})") from exc
                raise
            check_wanted(seeded)
            verify_library(service, route, home, downloads, ALIGNED_PAYLOADS)
            if torrent()["status"] != 6:
                raise RuntimeError("Transmission stopped seeding after library organization")
            verify_peer_upload(peer_port, infohash, ALIGNED_PAYLOADS[0][:PIECE_SIZE])
            verify_completed_task(service, infohash)
    print(f"Passed: Transmission {version}, RSS, selection, mid-download restart, library and verified peer upload.")


def verify_restore(binary, transmission_bin, root, offline, archive):
    home, backups, config = root / "app", root / "backups", root / "transmission"
    library, downloads, default = home / "剧集", root / "更新后 Downloads", root / "daemon-default"
    downloads.mkdir()
    rpc = Transmission(free_port())
    peer_port = configure_daemon(config, rpc, default)
    port, password = free_port(), secrets.token_urlsafe(32)
    source_sub, source_episode = archive[2:]
    payload = PAYLOADS[1]

    def app():
        return start([binary], home, backups, port, password, offline, env_overrides=env)

    def transfer(service, route, number):
        filename = NAMES[number - 1]
        infohash, _ = publish(Path(filename).stem, payload)
        torrent = wait_for("E02 seeding in Transmission", lambda: rpc.torrent(infohash),
                           lambda row: row and row["status"] == 6 and row["leftUntilDone"] == 0)
        if not Path(torrent["downloadDir"]).samefile(downloads) or any(default.iterdir()):
            raise RuntimeError("Transmission did not use the restored subscription's new download directory")
        detail = wait_for("restored E02 organized", lambda: service.api(route),
                          lambda d: any(ep["torrent_hash"] == infohash and ep["status"] == "organized" for ep in d["episodes"]))
        episode, = [ep for ep in detail["episodes"] if ep["number"] == number]
        final = Path(episode["final_path"])
        original = downloads / filename
        if (episode["torrent_hash"] != infohash or episode["file_size"] != len(payload) or original.is_symlink()
                or final.is_symlink() or not final.resolve().is_relative_to(library.resolve())
                or not final.samefile(original) or original.read_bytes() != payload):
            raise RuntimeError("The native E02 download and hard-linked library file differ from the reference")
        return episode

    with movie_feed(TITLE, http_seed=True) as (url, publish):
        env = app_environment(rpc, url, "/RSS/MyBangumi")
        with daemon(transmission_bin, config, rpc) as version:
            import_backup(app, transfer, archive, backups, library, downloads)

    # 此时 RSS 和 Web Seed 已关闭。只允许新对等端从重启后的 Transmission 取回 E02。
    with daemon(transmission_bin, config, rpc), app() as service:
        route = verify_history(service, source_sub["ID"], source_sub, source_episode, downloads, 2)
        episode, = [ep for ep in service.api(route)["episodes"] if ep["number"] == 2 and ep["status"] == "organized"]
        infohash = episode["torrent_hash"]
        torrent = wait_for("E02 seeding after both restarts", lambda: rpc.torrent(infohash),
                           lambda row: row and row["status"] == 6 and row["leftUntilDone"] == 0)
        if not Path(torrent["downloadDir"]).samefile(downloads) or not Path(episode["final_path"]).samefile(downloads / NAMES[1]):
            raise RuntimeError("The new directory and E02 library link did not survive restart")
        verify_peer_upload(peer_port, infohash, payload)
        verify_completed_task(service, infohash)
    print(f"Passed: Windows backup to Transmission {version}, new download directory and full E02 upload after restart.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path, help="Built Ani-Go executable")
    parser.add_argument("--daemon", default="transmission-daemon", help="Transmission daemon executable name or path")
    parser.add_argument("--restore-windows-backup", type=Path, metavar="ARCHIVE_DIR",
                        help="Restore the native Windows full backup and seed the new E02 after restarting")
    args = parser.parse_args()
    daemon_path = shutil.which(args.daemon)
    if not daemon_path:
        parser.error("transmission-daemon executable not found")
    with tempfile.TemporaryDirectory(prefix="Ani-Go Transmission ") as folder, socket.socket() as blocked:
        blocked.bind((HOST, 0))
        offline = f"http://{HOST}:{blocked.getsockname()[1]}"
        binary = str(args.binary.resolve(strict=True))
        daemon_bin = str(Path(daemon_path).resolve())
        if args.restore_windows_backup:
            archive = load_backup(args.restore_windows_backup, required_source="Windows")
            verify_restore(binary, daemon_bin, Path(folder), offline, archive)
            return
        media_ready = Event()
        with web_seed(media_ready, payloads=ALIGNED_PAYLOADS) as (url, infohash):
            verify(binary, daemon_bin, Path(folder), offline, url, infohash, media_ready)


if __name__ == "__main__":
    main()
