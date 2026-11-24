"""用真实 qB 验收同季两种清晰度的合集、原任务恢复及预分配文件的入库边界。"""

import argparse
from contextlib import contextmanager
import hashlib
from http.server import ThreadingHTTPServer
from pathlib import Path
import secrets
import shutil
import tempfile
from threading import Thread
from xml.sax.saxutils import escape

from test_platform_runtime import HOST, start
from test_qb_runtime import QB
from torrent_fixture import (NAMES, PAYLOADS, PIECE_SIZE, TITLE, SeedHandler, bencode,
                            create_pack_subscription, free_port, wait_for)


@contextmanager
def editions():
    with ThreadingHTTPServer((HOST, 0), SeedHandler) as server:
        url = f"http://{HOST}:{server.server_port}"
        server.media, server.media_ready = {}, None
        items, packs = [], {}
        for quality in ("2160p", "1080p"):
            title = f"[Fixture] {TITLE} [01-03] [{quality}]"
            data = [hashlib.shake_256((quality + name).encode()).digest(len(payload))
                    for name, payload in zip(NAMES, PAYLOADS)]
            content = b"".join(data)
            info = {b"name": title.encode(), b"files": [{b"length": len(blob), b"path": [name.encode()]}
                                                       for name, blob in zip(NAMES, data)],
                    b"piece length": PIECE_SIZE, b"private": 1,
                    b"pieces": b"".join(hashlib.sha1(content[i:i+PIECE_SIZE]).digest()
                                        for i in range(0, len(content), PIECE_SIZE))}
            digest = hashlib.sha1(bencode(info)).hexdigest()
            path = f"/{quality}.torrent"
            server.media[path] = bencode({b"info": info})
            items.append(f"<item><title>{escape(title)}</title><link>{url}/Home/Episode/{digest}</link>"
                         f'<enclosure url="{url}{path}" length="{len(server.media[path])}" '
                         'type="application/x-bittorrent"/></item>')
            packs[quality] = (title, digest, path, data)
        server.rss = f"<rss version='2.0'><channel><title>{TITLE}</title>{''.join(items)}</channel></rss>".encode()
        worker = Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            yield url, packs
        finally:
            server.shutdown()
            worker.join()


def library_versions(detail, packs, qualities, downloads, library):
    if detail["subscription"]["current_episodes"] != len(NAMES):
        raise RuntimeError("Progress must count distinct episodes across releases")
    rows = [row for row in detail["episodes"] if row["status"] == "organized"]
    if len(rows) != len(qualities) * len(NAMES):
        raise RuntimeError("Unexpected number of organized edition members")
    targets = set()
    ids = {}
    for quality in qualities:
        title, digest, _, samples = packs[quality]
        members = sorted((row for row in rows if row["torrent_hash"] == digest), key=lambda row: row["number"])
        if [row["number"] for row in members] != list(range(1, len(NAMES) + 1)):
            raise RuntimeError("Missing or duplicate episodes in " + quality)
        ids[quality] = [(row["id"], row["final_path"]) for row in members]
        for row, name, expected in zip(members, NAMES, samples):
            file = Path(row["final_path"])
            if (file.is_symlink() or not file.resolve().is_relative_to(library.resolve())
                    or file in targets or not file.samefile(downloads / title / name)
                    or file.read_bytes() != expected or row["file_size"] != len(expected)):
                raise RuntimeError(f"Incorrect library bytes, ownership or size for {quality} {name}")
            targets.add(file)
    if set(library.rglob("*.mkv")) != targets:
        raise RuntimeError("Library contains extra media, including a possible preallocated file")
    return ids


def run(binary, program):
    with tempfile.TemporaryDirectory(prefix="Ani-Go editions ") as folder, editions() as (url, packs):
        root = Path(folder)
        home, downloads = root / "app", root / "downloads"
        destination = QB(program, root / "primary-profile", downloads)
        seed = QB(program, root / "seed-profile", root / "private seed")
        for quality, (title, _, _, blobs) in packs.items():
            path = downloads / title
            path.mkdir(parents=True)
            for idx, (name, blob) in enumerate(zip(NAMES, blobs)):
                if quality == "2160p":
                    (path / name).write_bytes(blob)
                else:
                    with (path / name).open("wb") as f:
                        f.truncate(len(blob))
                        if idx == 0:
                            f.write(blob[:PIECE_SIZE])
        low_name, low_hash, low_path, low_blobs = packs["1080p"]
        high_hash = packs["2160p"][1]
        seed_dir = seed.downloads / low_name
        seed_dir.mkdir(parents=True)
        for name, blob in zip(NAMES, low_blobs):
            (seed_dir / name).write_bytes(blob)

        settings = dict(DEFAULT_DOWNLOADER="qbittorrent", QB_HOST=destination.url, QB_USER="admin",
                        QB_PASS=destination.password, RSS_INTERVAL="1s", ORGANIZER_INTERVAL="1s",
                        MIKAN_RSS_MODE="personal", MIKAN_DOMAIN=url, MIKAN_MIRROR_DOMAINS=url,
                        MIKAN_PROXY_DOMAIN=url, MIKAN_RSS_URL=url+"/RSS/Bangumi?bangumiId=42",
                        DISK_MIN_FREE_GB="0", USE_HARDLINK="true", SEED_CLEANUP_ENABLED="false")
        with (destination.running(),
              start([str(binary)], home, root / "backups", free_port(), secrets.token_urlsafe(24),
                    "http://127.0.0.1:9", env_overrides=settings) as service):
            print("Native qBittorrent", destination.request("app/version").decode().strip(), flush=True)
            route = create_pack_subscription(service, url, downloads)
            wait_for("RSS completed 2160p torrent", lambda: destination.info(high_hash),
                     lambda task: task.get("progress") == 1)
            partial = wait_for("one verified piece in preallocated torrent", lambda: destination.info(low_hash),
                               lambda task: task.get("completed") == PIECE_SIZE and "checking" not in task["state"].lower())
            detail = wait_for("complete edition organized while incomplete edition waits", lambda: service.api(route),
                              lambda d: sum(r["status"] == "organized" for r in d["episodes"]) == len(NAMES))
            first_ids = library_versions(detail, packs, ["2160p"], downloads, home / "剧集")
            pending, = [r for r in detail["episodes"] if r["torrent_hash"] == low_hash]
            if pending["status"] != "downloading":
                raise RuntimeError("Preallocated files look complete to Ani-Go")
            print(f"2160p was organized; 1080p retained {partial['completed']} verified bytes.", flush=True)

            destination.request("torrents/delete", {"hashes": high_hash+"|"+low_hash, "deleteFiles": "false"})
            wait_for("online downloader reporting both hashes absent", lambda: service.api("downloads"), lambda tasks: not tasks)
            missing = service.api(route)
            if service.api(route + "/download-tasks")["tasks"] != {low_hash: "missing"}:
                raise RuntimeError("Expected only the unfinished torrent to be reported missing")
            if library_versions(missing, packs, ["2160p"], downloads, home / "剧集") != first_ids:
                raise RuntimeError("Removing downloader tasks changed the completed library")
            if not any(r["id"] == pending["id"] and r["torrent_hash"] == low_hash for r in missing["episodes"]):
                raise RuntimeError("Removing the incomplete torrent erased the subscription's download history")
            for title, _, path, _ in packs.values():
                service.api("downloads", "POST", {"title": title, "url": url+path, "save_path": str(downloads)})
            wait_for("same 2160p hash restored", lambda: destination.info(high_hash), lambda task: task.get("progress") == 1)
            wait_for("same 1080p hash restored from retained files", lambda: destination.info(low_hash),
                     lambda task: task.get("completed") == PIECE_SIZE and "checking" not in task["state"].lower())
            restored = service.api(route)
            if service.api(route + "/download-tasks")["tasks"] != {low_hash: "present"}:
                raise RuntimeError("The restored task is still reported missing")
            remaining, = [r for r in restored["episodes"] if r["torrent_hash"] == low_hash]
            if remaining["id"] != pending["id"] or remaining["status"] != "downloading":
                raise RuntimeError("The restored incomplete task lost its history or entered the library")

            with seed.running():
                seed.add(url, low_path)
                wait_for("1080p loopback seed ready", lambda: seed.info(low_hash), lambda task: task.get("progress") == 1)
                destination.download_from(seed, low_hash, {Path(low_name) / n: b for n, b in zip(NAMES, low_blobs)})
            def both_editions(detail):
                if not any(r["id"] == pending["id"] for r in detail["episodes"]):
                    raise RuntimeError("Organizing a second release erased its original history")
                return sum(r["status"] == "organized" for r in detail["episodes"]) == 2*len(NAMES)

            complete = wait_for("both editions organized", lambda: service.api(route), both_editions)
            if service.api(route + "/download-tasks")["tasks"]:
                raise RuntimeError("Completed editions should no longer need a downloader check")
            ids = library_versions(complete, packs, ["2160p", "1080p"], downloads, home / "剧集")
            if ids["2160p"] != first_ids["2160p"] or pending["id"] not in {i for i, _ in ids["1080p"]}:
                raise RuntimeError("Organizing the second edition changed the history of the original releases")
            print("Passed: both original hashes and episode histories restored; 6 distinct hard links, 3 episodes.", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--qbittorrent", default="qbittorrent-nox")
    args = parser.parse_args()
    program = shutil.which(args.qbittorrent)
    if not program:
        parser.error("qBittorrent executable not found")
    run(args.binary.resolve(strict=True), program)
