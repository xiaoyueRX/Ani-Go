"""在两个可写文件系统上验证跨卷复制、媒体卷标恢复及 qBittorrent 重启后的独立做种。"""

import argparse
from contextlib import contextmanager
import errno
import hashlib
import json
import os
from pathlib import Path
import platform
import secrets
import socket
import tempfile

from test_platform_runtime import HOST, start
from test_qb_runtime import QB
from torrent_fixture import NAMES, PAYLOADS, TITLE, free_port, movie_feed, wait_for


FILENAME, PAYLOAD = NAMES[0], PAYLOADS[0]
EXPECTED_SHA = hashlib.sha256(PAYLOAD).hexdigest()


def probe_link(downloads, library):
    witness = downloads / "link-probe"
    witness.write_bytes(b"probe")
    try:
        os.link(witness, library / "link-probe")
    except OSError as exc:
        winerror = getattr(exc, "winerror", None)
        if exc.errno != errno.EXDEV and winerror != 17:
            raise
        if downloads.stat().st_dev == library.stat().st_dev:
            raise RuntimeError("The download and library directories report the same device") from exc
        return exc.errno, winerror
    finally:
        witness.unlink()
    raise RuntimeError("The hard-link probe succeeded; supply a different filesystem for the library")


def media_file(path):
    if path.is_symlink() or not path.is_file():
        raise RuntimeError("Expected a regular media file")
    info = path.stat()
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    if info.st_size != len(PAYLOAD) or digest != EXPECTED_SHA:
        raise RuntimeError("A media file differs from the synthetic release")
    return {"device": info.st_dev, "inode": info.st_ino, "bytes": info.st_size, "sha256": digest}


def ports_closed(client):
    for port in (client.web_port, client.peer_port):
        with socket.socket() as probe:
            probe.settimeout(1)
            if probe.connect_ex((HOST, port)) == 0:
                return False
    return True


@contextmanager
def occupy_peer_port(client, *, listen):
    """占用原选端口，验证 qB 只使用自己成功监听的新端口。"""
    with socket.socket() as occupied:
        if os.name == "nt":
            occupied.setsockopt(socket.SOL_SOCKET, socket.SO_EXCLUSIVEADDRUSE, 1)
        occupied.bind((HOST, 0))
        if listen:
            occupied.listen()
        port = occupied.getsockname()[1]
        client.peer_port = port
        with client.running():
            if client.peer_port == port:
                raise RuntimeError(f"qBittorrent advertised occupied peer port {port} (listening={listen})")
            print(f"qBittorrent rebound occupied peer port {port} to {client.peer_port} (listening={listen}).", flush=True)
            yield


def organized(detail):
    episodes = detail["episodes"]
    return (len(episodes) == 1 and episodes[0]["status"] == "organized"
            and detail["subscription"]["current_episodes"] == 1)


def wait_for_blocked_episode(service, route, marker, torrent_hash):
    def check():
        detail = service.api(route)
        if len(detail["episodes"]) != 1:
            raise RuntimeError("Expected exactly one episode for the submitted release")
        episode, = detail["episodes"]
        if detail["subscription"]["current_episodes"] != 0 or episode["final_path"] or episode["status"] == "organized":
            raise RuntimeError("Ani-Go published an episode while the library marker was wrong")
        if episode["status"] != "downloaded":
            return None
        # 等到本次整理确实因为卷标被拒绝，以免将正常的下载到整理间隔误判为保护生效。
        lines = service.api("logs?lines=100")["lines"]
        if not any(marker.name in line and Path(FILENAME).stem in line for line in lines):
            return None
        if episode["number"] != 1 or episode["torrent_hash"].lower() != torrent_hash:
            raise RuntimeError("The volume marker rejection belongs to a different episode")
        return {"id": episode["id"], "status": episode["status"], "current_episodes": detail["subscription"]["current_episodes"]}

    return wait_for("organizer to reject the mismatched library marker", check, bool)


def run(binary, qb, root, library, feed, publish, *, check_marker=False):
    primary = QB(qb, root / "primary", root / "下载 Downloads")
    seed = QB(qb, root / "seed", root / "seed media")
    receiver = QB(qb, root / "receiver", root / "received media")
    for client in (primary, seed, receiver):
        client.downloads.mkdir()
    link_errno, link_winerror = probe_link(primary.downloads, library)
    (seed.downloads / FILENAME).write_bytes(PAYLOAD)
    env = dict(DEFAULT_DOWNLOADER="qbittorrent", QB_HOST=primary.url, QB_USER="admin", QB_PASS=primary.password,
               MIKAN_RSS_MODE="personal", MIKAN_RSS_URL=feed + "/RSS/MyBangumi", MIKAN_DOMAIN=feed,
               MIKAN_MIRROR_DOMAINS=feed, MIKAN_PROXY_DOMAIN=feed, RSS_INTERVAL="1s", ORGANIZER_INTERVAL="1s",
               TV_BASE_PATH=str(library), USE_HARDLINK="true", DISK_MIN_FREE_GB="0", SEED_CLEANUP_ENABLED="false")
    if check_marker:
        marker = library / ".anigo-volume-id"
        marker.write_bytes(b"other-library\r\n")
        env["TV_VOLUME_ID"] = "test-library"
    port, password = free_port(), secrets.token_urlsafe(32)

    def app():
        return start([str(binary)], root / "app", root / "backups", port, password, "http://127.0.0.1:9", env_overrides=env)

    with primary.running(), app() as service:
        sub = service.api("subscriptions", "POST", {"title_cn": TITLE, "custom_path": str(primary.downloads)}, status=201)
        route = f"subscriptions/{sub['id']}"
        service.api(route, "PUT", {"total_episodes": 1})
        with (occupy_peer_port(seed, listen=False) if check_marker else seed.running()):
            torrent_hash, torrent_path = publish(Path(FILENAME).stem, PAYLOAD)
            seed.add(feed, torrent_path)
            wait_for("source qBittorrent hash check", lambda: seed.info(torrent_hash), lambda row: row.get("progress") == 1)
            job = wait_for("RSS transfer submitted to qBittorrent", lambda: primary.info(torrent_hash), bool)
            if not Path(job["save_path"]).samefile(primary.downloads):
                raise RuntimeError("qBittorrent received the wrong download directory")
            primary.download_from(seed, torrent_hash, {FILENAME: PAYLOAD})
        wait_for("original seeder to close its Web UI and peer port", lambda: ports_closed(seed), bool)

        source = primary.downloads / FILENAME
        if check_marker:
            blocked = wait_for_blocked_episode(service, route, marker, torrent_hash)
            blocked_source = media_file(source)
            if list(library.iterdir()) != [marker] or marker.read_bytes() != b"other-library\r\n":
                raise RuntimeError("The organizer changed the library while its marker was wrong")
            marker.write_bytes(b"test-library\r\n")
        detail = wait_for("library copy and episode progress", lambda: service.api(route), organized)
        episode, = detail["episodes"]
        copy = Path(episode["final_path"])
        if (episode["number"] != 1 or episode["torrent_hash"].lower() != torrent_hash or episode["file_size"] != len(PAYLOAD)
                or not copy.resolve().is_relative_to(library.resolve()) or source.samefile(copy)):
            raise RuntimeError("The organized episode is not a separate copy in the selected media library")
        source_file, library_file = media_file(source), media_file(copy)
        if source_file["device"] == library_file["device"]:
            raise RuntimeError("The source and organized media occupy the same device")
        if check_marker and (episode["id"] != blocked["id"] or source_file != blocked_source):
            raise RuntimeError("Automatic recovery replaced the waiting episode or changed the completed download")
    wait_for("primary qBittorrent to close before restart", lambda: ports_closed(primary), bool)

    # 此时原始种源已退出。私有种子没有 tracker 或 Web Seed，空接收端只连接重新启动的主下载器。
    receiver_run = occupy_peer_port(receiver, listen=True) if check_marker else receiver.running()
    with primary.running(), app() as restarted, receiver_run:
        job = wait_for("completed torrent restored after restart", lambda: primary.info(torrent_hash),
                       lambda row: row.get("progress") == 1)
        if not Path(job["content_path"]).samefile(source):
            raise RuntimeError("The restarted qBittorrent torrent no longer points at the original download")
        detail = restarted.api(route)
        restored, = detail["episodes"]
        if (not organized(detail) or restored["id"] != episode["id"] or restored["final_path"] != episode["final_path"]
                or restored["torrent_hash"].lower() != torrent_hash
                or media_file(source) != source_file or media_file(copy) != library_file):
            raise RuntimeError("Restart changed the episode, download source or library copy")
        if receiver.info(torrent_hash) or (receiver.downloads / FILENAME).exists():
            raise RuntimeError("The independent receiver already has the test release")
        before = primary.info(torrent_hash)["uploaded_session"]
        receiver.add(feed, torrent_path)
        wait_for("independent receiver torrent", lambda: receiver.info(torrent_hash), bool)
        receiver.download_from(primary, torrent_hash, {FILENAME: PAYLOAD})
        uploaded = wait_for("primary qBittorrent upload counter", lambda: primary.info(torrent_hash)["uploaded_session"] - before,
                            lambda count: count >= len(PAYLOAD))
        received_file = media_file(receiver.downloads / FILENAME)
        result = {
            "system": platform.system(), "architecture": platform.machine(),
            "qb_version": primary.request("app/version").decode().strip(), "infohash": torrent_hash,
            "link_errno": link_errno, "link_winerror": link_winerror,
            "source": source_file, "library": library_file, "receiver": received_file,
            "origin_offline": True, "anigo_restarted": True, "qb_restarted": True, "uploaded_delta": uploaded,
        }
        if check_marker:
            result["volume_marker"] = {
                "blocked": blocked, "recovered_episode_id": restored["id"],
                "recovered_progress": detail["subscription"]["current_episodes"],
            }
        return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path, help="Built Ani-Go executable")
    parser.add_argument("qbittorrent", type=Path, help="Native qBittorrent GUI or nox executable")
    parser.add_argument("library_parent", type=Path, help="Writable directory on a filesystem distinct from the default temp directory")
    args = parser.parse_args()
    binary, qb = args.binary.resolve(strict=True), str(args.qbittorrent.resolve(strict=True))
    for check_marker, label in ((False, "Cross-volume qBittorrent"), (True, "Cross-volume marker recovery")):
        with (tempfile.TemporaryDirectory(prefix="Ani-Go downloads ") as root,
              tempfile.TemporaryDirectory(prefix="Ani-Go library ", dir=args.library_parent.resolve(strict=True)) as library,
              movie_feed(TITLE) as (feed, publish)):
            result = run(binary, qb, Path(root), Path(library), feed, publish, check_marker=check_marker)
        print(label + ": " + json.dumps(result), flush=True)


if __name__ == "__main__":
    main()
