"""在独立数据目录中从官方 v0.7.0 手动升级，验证旧账号、订阅、设置和插件。"""

import argparse
from contextlib import closing, contextmanager
from http.client import HTTPException
import os
from pathlib import Path
import secrets
import shutil
import signal
import socket
import sqlite3
import subprocess
import tempfile
from urllib.parse import quote

from test_platform_runtime import HOST, Service, process_environment
from test_update_runtime import sha256
from torrent_fixture import free_port, wait_for


LEGACY_VERSION = "v0.7.0"
OFFLINE = f"http://{HOST}:9"
RSS_URL = OFFLINE + "/synthetic.xml"
SETTINGS = {"BACKUP_KEEP_COUNT": "9", "BACKUP_CRON": "0 23 * * *"}
PLUGIN = {"id": "legacy-hook", "name": "旧版合成通知", "type": "webhook",
          "url": "https://legacy-fixture.invalid/webhook", "events": ["download.completed"]}
PLUGIN_TOGGLE = "plugins/" + PLUGIN["id"] + "/toggle"


def ensure_windows_console():
    # 无控制台的 Windows CI 进程需要一个控制台来向专属进程组发送 Ctrl+Break。
    if os.name == "nt":
        import ctypes
        kernel = ctypes.windll.kernel32
        if not kernel.GetConsoleCP() and not kernel.AllocConsole():
            raise ctypes.WinError()


@contextmanager
def running(binary, home, env, service):
    flags = subprocess.CREATE_NEW_PROCESS_GROUP if os.name == "nt" else 0
    with subprocess.Popen([str(binary)], cwd=home, env=env, creationflags=flags,
                          stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL) as proc:
        def health():
            if proc.poll() is not None:
                raise RuntimeError(f"Ani-Go exited during startup with code {proc.returncode}")
            try:
                return service.api("health")
            except (OSError, HTTPException):
                return {}

        try:
            yield wait_for("Ani-Go health", health, lambda result: result.get("status") == "ok")
        finally:
            try:
                if proc.poll() is None:
                    proc.send_signal(signal.CTRL_BREAK_EVENT if os.name == "nt" else signal.SIGTERM)
                    proc.wait(timeout=20)
            finally:
                if proc.poll() is None:
                    proc.kill()
                    proc.wait()
            if proc.returncode != 0:
                raise RuntimeError(f"Ani-Go did not shut down cleanly (code {proc.returncode})")
            with socket.socket() as probe:
                probe.settimeout(1)
                if probe.connect_ex((HOST, service.port)) == 0:
                    raise RuntimeError("Ani-Go still has its original port open after shutdown")


def verify_records(service, expected, plugin_enabled, *, candidate):
    if service.api("me").get("username") != "admin":
        raise RuntimeError("The previous session cannot identify the administrator")
    rows = service.api("subscriptions")
    if len(rows) != 1 or rows[0]["id"] != expected["id"]:
        raise RuntimeError("The legacy subscription ID or count changed")
    detail = service.api(f"subscriptions/{expected['id']}")["subscription"]
    for key, value in expected.items():
        if detail.get(key) != value:
            raise RuntimeError(f"The legacy subscription differs in {key}")
    saved = service.api("settings")
    if any(saved.get(key) != value for key, value in SETTINGS.items()):
        raise RuntimeError("The backup schedule or retention count changed")
    plugins = service.api("plugins/custom")
    if len(plugins) != 1 or any(plugins[0].get(key) != value for key, value in PLUGIN.items()):
        raise RuntimeError("The legacy webhook manifest changed")
    if plugins[0]["enabled"] != plugin_enabled:
        raise RuntimeError("The legacy webhook lost its enabled state")
    if candidate:
        status = "active" if plugin_enabled else "inactive"
        if plugins[0]["origin"] != "community" or plugins[0]["status"] != status:
            raise RuntimeError("The candidate did not apply the legacy webhook state")


def verify_database(database, subscription_id):
    # v0.7.0 正常退出后仍可能留下 WAL。以只读模式打开整套冷备验证内容。
    with closing(sqlite3.connect(database.as_uri() + "?mode=ro", uri=True)) as connection:
        if connection.execute("PRAGMA integrity_check").fetchall() != [("ok",)]:
            raise RuntimeError("SQLite integrity check failed")
        rows = connection.execute("SELECT id, rss_url FROM subscriptions").fetchall()
        if rows != [(subscription_id, RSS_URL)]:
            raise RuntimeError("The persisted subscription or RSS address is missing")


def prepare_legacy(service, password, media, backups):
    service.login("admin")
    version = service.api("version")["version"]
    if version != LEGACY_VERSION:
        raise RuntimeError(f"Expected the official {LEGACY_VERSION} legacy binary, found {version}")
    service.api("user/change-password", "POST", {"old_password": "admin", "new_password": password})
    service.login(password)
    expected = {"title_cn": "旧版订阅 Archive", "bangumi_id": "42", "custom_path": str(media),
                "subgroup_name": "样例字幕组"}
    created = service.api("subscriptions", "POST", dict(expected, rss_url=RSS_URL), status=201)
    changes = {"title_en": "Legacy archive", "season": 3, "enabled": False, "total_episodes": 12}
    service.api(f"subscriptions/{created['id']}", "PUT", changes)
    expected.update(changes, id=created["id"])
    service.api("settings", "PUT", {"settings": SETTINGS})
    service.api("plugins/save", "POST", PLUGIN)
    service.api(PLUGIN_TOGGLE, "POST", {"enabled": False})
    service.verify_web()
    verify_records(service, expected, False, candidate=False)
    service.api("backup/create", "POST", {"include_episodes": True})
    entries = service.api("backup/list")
    if len(entries) != 1:
        raise RuntimeError("The legacy release did not produce its administrator backup")
    name = entries[0]["name"]
    _, downloaded = service.request("/api/backup/download/" + quote(name, safe=""))
    if not downloaded or downloaded != (backups / name).read_bytes():
        raise RuntimeError("The legacy backup download differs from the file on disk")
    return expected


def run(legacy, candidate, root):
    service = Service(free_port())
    data = root / "persistent"
    home, backups, media = data / "home", data / "backups", data / "合成 目录"
    for path in (home, backups, media):
        path.mkdir(parents=True)
    binary = root / ("anigo.exe" if os.name == "nt" else "anigo")
    shutil.copy2(legacy, binary)
    password = secrets.token_urlsafe(24)
    env = process_environment(home, backups, service.port, password, OFFLINE)
    del env["ANIGO_ADMIN_PASSWORD"]
    database = Path(env["ANI_DB_PATH"])
    print(f"Official {LEGACY_VERSION} executable SHA-256 {sha256(binary)}", flush=True)
    with running(binary, home, env, service):
        expected = prepare_legacy(service, password, media, backups)
        legacy_token = service.token

    snapshot = root / "cold backup"
    shutil.copytree(data, snapshot)
    shutil.copy2(binary, snapshot / binary.name)
    verify_database(snapshot / database.relative_to(data), expected["id"])
    print("Stopped the legacy service and verified its complete cold backup.", flush=True)

    replacement = root / "anigo.new"
    shutil.copy2(candidate, replacement)
    replacement.replace(binary)
    print(f"Candidate executable SHA-256 {sha256(binary)}", flush=True)
    with running(binary, home, env, service) as health:
        # 先用旧进程颁发的 JWT 验证，再以旧管理员修改过的密码登录。
        verify_records(service, expected, False, candidate=True)
        if not health.get("version") or service.api("version")["version"] != health["version"]:
            raise RuntimeError("The upgraded process did not report its version")
        service.verify_web()
        service.api("login", "POST", {"username": "admin", "password": "admin"}, status=401)
        service.login(password)
        service.api(PLUGIN_TOGGLE, "POST", {"enabled": True})
        verify_records(service, expected, True, candidate=True)

    with running(binary, home, env, service):
        verify_records(service, expected, True, candidate=True)
        service.api(PLUGIN_TOGGLE, "POST", {"enabled": False})
        verify_records(service, expected, False, candidate=True)
    verify_database(database, expected["id"])
    print(f"Upgraded in place to {health['version']}. The legacy session, changed password, subscription, "
          "settings and plugin passed, including a candidate restart.", flush=True)

    restored = root / "restored legacy"
    shutil.copytree(snapshot, restored)
    restored_home = restored / home.relative_to(data)
    rollback_env = process_environment(restored_home, restored / backups.relative_to(data), service.port, password, OFFLINE)
    del rollback_env["ANIGO_ADMIN_PASSWORD"]
    service.token = legacy_token
    with running(restored / binary.name, restored_home, rollback_env, service):
        if service.api("version")["version"] != LEGACY_VERSION:
            raise RuntimeError("The restored executable did not report the legacy version")
        verify_records(service, expected, False, candidate=False)
        service.login(password)
        service.verify_web()
    print(f"Restored {LEGACY_VERSION} in a separate directory and verified the original login and records.", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path, help="Candidate executable")
    parser.add_argument("--legacy", type=Path, required=True, help="Official v0.7.0 executable")
    args = parser.parse_args()
    ensure_windows_console()
    with tempfile.TemporaryDirectory(prefix="Ani-Go legacy upgrade ") as temp:
        run(args.legacy.resolve(strict=True), args.binary.resolve(strict=True), Path(temp).resolve())


if __name__ == "__main__":
    main()
