"""跨原生系统导出、恢复完整管理员 JSON，并通过真实 qBittorrent 为恢复的订阅补集。"""

import argparse
from contextlib import ExitStack
from functools import partial
import hashlib
import json
from pathlib import Path, PurePosixPath, PureWindowsPath
import platform
import secrets
import shutil
import tempfile
from urllib.parse import quote

from test_platform_runtime import start
from test_qb_runtime import QB
from torrent_fixture import TITLE, NAMES, PAYLOADS, create_pack_subscription, free_port, movie_feed, wait_for, web_seed


SEASON = "Season 1"
LOCAL_TITLE = "目标已有订阅"
OFFLINE = "http://127.0.0.1:9"


def prepare_pack(primary, seed, url, infohash):
    folder = seed.downloads / TITLE / SEASON
    folder.mkdir(parents=True)
    for name, payload in zip(NAMES, PAYLOADS):
        (folder / name).write_bytes(payload)
    wait_for("RSS pack in qBittorrent", lambda: primary.info(infohash), bool)
    route = "torrents/files?hash=" + infohash
    files = wait_for("native pack files", lambda: json.loads(primary.request(route)), lambda rows: len(rows) == 3)
    if [Path(row["name"]).name for row in files] != NAMES:
        raise RuntimeError("qBittorrent returned an unexpected pack")
    primary.request("torrents/filePrio", {"hash": infohash, "id": "|".join(str(row["index"]) for row in files[1:]), "priority": 0})
    wait_for("only nested E01 selected", lambda: json.loads(primary.request(route)),
             lambda rows: [row["priority"] > 0 for row in rows] == [True, False, False])
    seed.add(url)
    return infohash, Path(TITLE) / SEASON / NAMES[0]


def prepare_single(seed, url, publish):
    (seed.downloads / NAMES[1]).write_bytes(PAYLOADS[1])
    infohash, torrent = publish(Path(NAMES[1]).stem, PAYLOADS[1])
    seed.add(url, torrent)
    return infohash, Path(NAMES[1])


def organize(service, route, number, library, primary, seed, prepare):
    infohash, member = prepare()
    payload = PAYLOADS[number - 1]
    wait_for("source hash check", lambda: seed.info(infohash), lambda row: row.get("progress") == 1)
    task = wait_for("RSS delivery to qBittorrent", lambda: primary.info(infohash), bool)
    if not Path(task["save_path"]).samefile(primary.downloads):
        raise RuntimeError("qBittorrent did not receive the native download directory")
    primary.download_from(seed, infohash, {member: payload})

    def organized(detail):
        return any(ep["torrent_hash"].lower() == infohash and ep["status"] == "organized" for ep in detail["episodes"])

    detail = wait_for("library organization", lambda: service.api(route), organized)
    row, = [ep for ep in detail["episodes"] if ep["number"] == number]
    final = Path(row["final_path"])
    if (row["torrent_hash"].lower() != infohash or row["file_size"] != len(payload)
            or final.is_symlink() or not final.resolve().is_relative_to(library.resolve())
            or not final.samefile(primary.downloads / member) or final.read_bytes() != payload):
        raise RuntimeError("The native library does not contain the hard-linked synthetic release")
    return row


def export_backup(app, transfer, directory, downloads, url, passwords):
    directory.mkdir(parents=True, exist_ok=True)
    if any(directory.iterdir()):
        raise RuntimeError("The export artifact directory must be empty")
    with app() as service:
        route = create_pack_subscription(service, url, downloads)
        episode = transfer(service, route, 1)
        service.api("backup/create", "POST", {"include_episodes": True})
        entry, = service.api("backup/list")
        name = entry["name"]
        _, raw = service.request("/api/backup/download/" + quote(name, safe=""))
        snapshot = json.loads(raw)
        archived, = snapshot["episodes"]
        saved, = snapshot["subscriptions"]
        if (archived["FinalPath"] != episode["final_path"] or archived["TorrentHash"] != episode["torrent_hash"]
                or saved["CustomPath"] != str(downloads) or archived["Status"] != "organized"):
            raise RuntimeError("The full JSON differs from the media organized on the export host")
        if ("users" in snapshot or any(key.lower() == "jwt_secret" for key in snapshot["settings"])
                or any(value.encode() in raw for value in (*passwords, service.token))):
            raise RuntimeError("The generated fixture contains a test credential")
    digest = hashlib.sha256(raw).hexdigest()
    source = platform.system()
    verify_member_key(saved, archived, source)
    (directory / name).write_bytes(raw)
    manifest = {"archive": name, "source_os": source, "source_arch": platform.machine(), "sha256": digest}
    (directory / "manifest.json").write_text(json.dumps(manifest), encoding="utf-8")
    print(f"Portable backup export: source={source} sha256={digest} organized_episodes=1 nested_member_key=verified media_bytes={len(PAYLOADS[0])}")


def verify_member_key(sub, episode, source):
    path_type = PureWindowsPath if source == "Windows" else PurePosixPath
    member = path_type(TITLE, SEASON, NAMES[0])
    if episode["SourceFile"] != str(member):
        raise RuntimeError("The source episode does not contain a native nested torrent member")
    content = (episode["TorrentHash"].lower() + "\0" + member.as_posix()).encode()
    expected = f"{sub['ID']}/file/{hashlib.sha256(content).hexdigest()}"
    if episode.get("ReservationKey") != expected:
        raise RuntimeError("The archived E01 does not have its verified file reservation key")


def load_backup(directory, required_source=None):
    manifest = json.loads((directory / "manifest.json").read_bytes())
    source, name = manifest["source_os"], manifest["archive"]
    if source not in ("Linux", "Darwin", "Windows") or (required_source and source != required_source):
        raise RuntimeError("The artifact was not produced on the requested native system")
    archives = list(directory.glob("anigo_backup_*_full.json"))
    if len(archives) != 1 or archives[0].name != name:
        raise RuntimeError("The manifest must identify one full backup in the artifact")
    raw = archives[0].read_bytes()
    if hashlib.sha256(raw).hexdigest() != manifest["sha256"]:
        raise RuntimeError("The full JSON changed in transit")
    snapshot = json.loads(raw)
    sub, = snapshot["subscriptions"]
    episode, = snapshot["episodes"]
    path_type = PureWindowsPath if source == "Windows" else PurePosixPath
    if (not path_type(sub["CustomPath"]).is_absolute() or not path_type(episode["FinalPath"]).is_absolute()
            or not path_type(episode["DownloadRoot"]).is_absolute() or sub["TitleCN"] != TITLE or episode["SubscriptionID"] != sub["ID"]
            or episode["Status"] != "organized" or episode["Number"] != 1 or episode["FileSize"] != len(PAYLOADS[0])):
        raise RuntimeError("The archive does not contain the native source paths and organized E01")
    verify_member_key(sub, episode, source)
    return manifest, raw, sub, episode


def verify_history(service, local_id, old_sub, old_ep, expected_path, count):
    if service.api("me")["username"] != "admin":
        raise RuntimeError("The target administrator lost access after restore")
    subs = service.api("subscriptions")
    rows = {sub["title_cn"]: sub for sub in subs}
    if (len(subs) != 2 or set(rows) != {LOCAL_TITLE, TITLE} or rows[LOCAL_TITLE]["id"] != local_id
            or rows[LOCAL_TITLE]["enabled"] or rows[LOCAL_TITLE]["current_episodes"] != 0):
        raise RuntimeError("The restore duplicated subscriptions or replaced the pre-existing target subscription")
    restored = rows[TITLE]
    if (restored["id"] == old_sub["ID"] or restored["custom_path"] != str(expected_path)
            or restored["current_episodes"] != count or restored["total_episodes"] != 3):
        raise RuntimeError("Restored subscription identity, path or progress differs")
    route = f"subscriptions/{restored['id']}"
    episodes = service.api(route)["episodes"]
    historical, = [ep for ep in episodes if ep["number"] == 1]
    if (len(episodes) != count or historical["subscription_id"] != restored["id"] or historical["status"] != "organized"
            or historical["final_path"] != old_ep["FinalPath"] or historical["file_size"] != old_ep["FileSize"]
            or historical["torrent_hash"] != old_ep["TorrentHash"]):
        raise RuntimeError("The imported episode lost its original library path, identity or subscription")
    return route


def import_backup(app, transfer, archive, backups, library, downloads):
    manifest, raw, old_sub, old_ep = archive
    name = manifest["archive"]
    backups.mkdir(exist_ok=True)
    (backups / name).write_bytes(raw)
    with app() as first:
        local = first.api("subscriptions", "POST", {"title_cn": LOCAL_TITLE}, status=201)
        if local["id"] != old_sub["ID"]:
            raise RuntimeError("The fixture must exercise an occupied subscription ID")
        first.api(f"subscriptions/{local['id']}", "PUT", {"enabled": False})
        entry, = first.api("backup/list")
        _, downloaded = first.request("/api/backup/download/" + quote(name, safe=""))
        if entry["name"] != name or downloaded != raw:
            raise RuntimeError("The target backup endpoint did not receive the unmodified file")
        for _ in range(2):
            first.api("backup/restore", "POST", {"name": name})
            verify_history(first, local["id"], old_sub, old_ep, old_sub["CustomPath"], 1)
        if list(library.rglob("*.mkv")):
            raise RuntimeError("Restoring JSON unexpectedly created media in the target library")
    with app() as restarted:
        # 此处使用恢复前已签发的目标会话，重启登录另外验证本地账号口令仍有效。
        route = verify_history(first, local["id"], old_sub, old_ep, old_sub["CustomPath"], 1)
        restarted.api(route, "PUT", {"custom_path": str(downloads)})
        row = transfer(restarted, route, 2)
        verify_history(restarted, local["id"], old_sub, old_ep, downloads, 2)
        if [path.resolve() for path in library.rglob("*.mkv")] != [Path(row["final_path"]).resolve()]:
            raise RuntimeError("The destination library must contain only its newly downloaded E02")
        restarted.api("backup/create", "POST", {"include_episodes": True})
        fresh, = [entry["name"] for entry in restarted.api("backup/list") if entry["name"] != name]
        _, reexported = restarted.request("/api/backup/download/" + quote(fresh, safe=""))
        current = json.loads(reexported)
        current_sub, = [sub for sub in current["subscriptions"] if sub["TitleCN"] == TITLE]
        historical, = [ep for ep in current["episodes"] if ep["SubscriptionID"] == current_sub["ID"] and ep["Number"] == 1]
        old_path = PureWindowsPath if manifest["source_os"] == "Windows" else PurePosixPath
        member = Path(*old_path(old_ep["SourceFile"]).parts)
        expected_key = f"{current_sub['ID']}/" + old_ep["ReservationKey"].split("/", 1)[1]
        if (historical.get("ReservationKey") != expected_key or historical["SourceFile"] != str(member)
                or historical["DownloadRoot"] != old_ep["DownloadRoot"]):
            raise RuntimeError("Restored E01 lost its native member path, historical download root or remapped reservation key")
    print(f"Portable backup import: source={manifest['source_os']} destination={platform.system()} sha256={manifest['sha256']} "
          f"restores=2 target_session=preserved historical_paths=1 remapped_member_key=verified native_episode=2 media_bytes={len(PAYLOADS[1])}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("phase", choices=("export", "import"))
    parser.add_argument("binary", type=Path)
    parser.add_argument("artifact_dir", type=Path)
    parser.add_argument("--qbittorrent", help="Native qBittorrent or qBittorrent-nox executable")
    source = parser.add_mutually_exclusive_group()
    source.add_argument("--require-source", choices=("Linux", "Windows", "Darwin"), help="Require an archive from this native system")
    source.add_argument("--require-windows-source", dest="require_source", action="store_const", const="Windows",
                        help=argparse.SUPPRESS)
    args = parser.parse_args()
    if args.require_source and args.phase != "import":
        parser.error("The source-system requirement applies to import")
    program = (shutil.which(args.qbittorrent) if args.qbittorrent else shutil.which("qbittorrent-nox") or shutil.which("qbittorrent"))
    if not program:
        parser.error("qBittorrent executable not found")
    command = [str(args.binary.resolve(strict=True))]
    archive = load_backup(args.artifact_dir, args.require_source) if args.phase == "import" else None
    with tempfile.TemporaryDirectory(prefix="Ani-Go portable ") as folder, ExitStack() as feeds:
        root = Path(folder)
        home, backups = root / "本地 实例", root / "备份"
        library = home / "剧集"
        primary = QB(program, root / "primary", root / "本地 Downloads")
        seed = QB(program, root / "seed", root / "种源")
        primary.downloads.mkdir()
        seed.downloads.mkdir()
        if archive is None:
            url, infohash = feeds.enter_context(web_seed(http_seed=False, member_directory=SEASON))
            prepare = partial(prepare_pack, primary, seed, url, infohash)
        else:
            url, publish = feeds.enter_context(movie_feed(TITLE))
            prepare = partial(prepare_single, seed, url, publish)
        port, password = free_port(), secrets.token_urlsafe(32)
        env = dict(DEFAULT_DOWNLOADER="qbittorrent", QB_HOST=primary.url, QB_USER="admin", QB_PASS=primary.password,
                   RSS_INTERVAL="1s", ORGANIZER_INTERVAL="1s", MIKAN_RSS_MODE="personal", MIKAN_RSS_URL=url + "/RSS/MyBangumi",
                   MIKAN_DOMAIN=url, MIKAN_PROXY_DOMAIN=url, MIKAN_MIRROR_DOMAINS=url,
                   DISK_MIN_FREE_GB="0", USE_HARDLINK="true", SEED_CLEANUP_ENABLED="false")

        def app():
            return start(command, home, backups, port, password, OFFLINE, env_overrides=env)

        def transfer(service, route, number):
            return organize(service, route, number, library, primary, seed, prepare)

        with primary.running(), seed.running():
            if archive is None:
                export_backup(app, transfer, args.artifact_dir, primary.downloads, url, (password, primary.password, seed.password))
            else:
                import_backup(app, transfer, archive, backups, library, primary.downloads)


if __name__ == "__main__":
    main()
