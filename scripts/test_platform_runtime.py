"""在临时目录中验收当前平台的 Ani-Go 可执行文件、内嵌网页和备份恢复。"""

import argparse
from contextlib import contextmanager
from html.parser import HTMLParser
from http.client import HTTPConnection, HTTPException
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
import time
from urllib.parse import quote


HOST = "127.0.0.1"
TITLE = "平台验收 Archive"
CUSTOM_PATH = "下载 目录"


class WebEntry(HTMLParser):
    def __init__(self):
        super().__init__()
        self.bundle = None

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == "script" and attrs.get("type") == "module":
            self.bundle = attrs.get("src")


class Service:
    def __init__(self, port):
        self.port = port
        self.token = None

    def request(self, path, method="GET", payload=None, status=200):
        headers = {}
        body = None
        if self.token:
            headers["Authorization"] = "Bearer " + self.token
        if payload is not None:
            headers["Content-Type"] = "application/json"
            body = json.dumps(payload).encode()
        client = HTTPConnection(HOST, self.port, timeout=5)
        try:
            client.request(method, path, body, headers)
            response = client.getresponse()
            if response.status != status:
                raise RuntimeError(f"{method} {path} returned HTTP {response.status}, expected {status}")
            return response.getheader("Content-Type", ""), response.read()
        finally:
            client.close()

    def api(self, path, *args, **kwargs):
        _, body = self.request("/api/" + path, *args, **kwargs)
        return json.loads(body)

    def login(self, password):
        self.token = self.api("login", "POST", {"username": "admin", "password": password})["token"]

    def verify_web(self):
        mime, index = self.request("/")
        entry = WebEntry()
        entry.feed(index.decode("utf-8"))
        if "text/html" not in mime or not entry.bundle or not entry.bundle.startswith("/assets/"):
            raise RuntimeError("Embedded web page is missing its bundled module")
        mime, module = self.request(entry.bundle)
        if "javascript" not in mime or not module.strip():
            raise RuntimeError("Embedded JavaScript module is missing or has the wrong content type")


def process_environment(home, backups, port, password, offline):
    env = {key: os.environ[key] for key in ("PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "SYSTEMDRIVE", "LANG") if key in os.environ}
    env.update(
        HOME=str(home), USERPROFILE=str(home), TMPDIR=str(home), TEMP=str(home), TMP=str(home),
        HOST=HOST, ANI_PORT=str(port), ANI_DB_PATH=str(home / "订阅 数据.sqlite"),
        ANIGO_ADMIN_PASSWORD=password, BACKUP_PATH=str(backups),
        TV_BASE_PATH=str(home / "剧集"), MOVIE_BASE_PATH=str(home / "电影"), OVA_BASE_PATH=str(home / "番外"),
        QB_HOST=offline, MIKAN_DOMAIN=offline, MIKAN_MIRROR_DOMAINS=offline, MIKAN_PROXY_DOMAIN=offline,
        MIKAN_RSS_URL=offline + "/rss", BGMTV_MIRROR_DOMAINS=offline, TMDB_MIRROR_DOMAINS=offline,
        HTTP_PROXY=offline, HTTPS_PROXY=offline, NO_PROXY=HOST, RSS_INTERVAL="24h",
    )
    return env


@contextmanager
def start(command, home, backups, port, password, offline):
    home.mkdir(exist_ok=True)
    service = Service(port)
    env = process_environment(home, backups, port, password, offline)
    proc = subprocess.Popen(command, cwd=home, env=env, stdin=subprocess.DEVNULL,
                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        deadline = time.monotonic() + 90
        while time.monotonic() < deadline:
            if proc.poll() is not None:
                raise RuntimeError(f"Ani-Go exited before startup with status {proc.returncode}")
            try:
                health = service.api("health")
            except (OSError, HTTPException):
                time.sleep(0.15)
                continue
            if health.get("status") != "ok" or not health.get("version"):
                raise RuntimeError("Health endpoint did not report a running version")
            break
        else:
            raise RuntimeError("Timed out waiting for the loopback health endpoint")
        service.login(password)
        yield service
    finally:
        if proc.poll() is None:
            proc.terminate()
            try:
                proc.wait(timeout=20)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait()
                raise RuntimeError("Ani-Go did not stop within 20 seconds")
    with socket.socket() as probe:
        probe.settimeout(1)
        if probe.connect_ex((HOST, port)) == 0:
            raise RuntimeError("The HTTP listener is still open after process exit")


def verify_subscription(service, custom_path):
    rows = service.api("subscriptions")
    if len(rows) != 1:
        raise RuntimeError(f"Expected one restored subscription, found {len(rows)}")
    detail = service.api(f"subscriptions/{rows[0]['id']}")["subscription"]
    expected = dict(title_cn=TITLE, season=2, enabled=False, total_episodes=7, custom_path=str(custom_path))
    for field, value in expected.items():
        if detail.get(field) != value:
            raise RuntimeError(f"Restored subscription differs in {field}")


def run(command, root, port, offline):
    password = secrets.token_urlsafe(32)
    backups = root / "共享 备份"
    backups.mkdir()
    custom_path = root / CUSTOM_PATH
    custom_path.mkdir()
    target = root / "恢复 实例"
    with start(command, root / "原始 实例", backups, port, password, offline) as service:
        service.verify_web()
        sub = service.api("subscriptions", "POST", {
            "title_cn": TITLE, "season": 2, "bangumi_id": "42", "rss_url": offline + "/rss", "custom_path": str(custom_path),
        }, status=201)
        service.api(f"subscriptions/{sub['id']}", "PUT", {"enabled": False, "total_episodes": 7})
        service.api("backup/create", "POST", {"include_episodes": True})
        entries = service.api("backup/list")
        if len(entries) != 1:
            raise RuntimeError("Expected exactly one newly created backup")
        name = entries[0]["name"]
        _, data = service.request("/api/backup/download/" + quote(name, safe=""))
        if not data or (backups / name).read_bytes() != data:
            raise RuntimeError("Downloaded backup differs from the file on disk")
        if len(json.loads(data)["subscriptions"]) != 1:
            raise RuntimeError("Backup does not contain the source subscription")
    with start(command, target, backups, port, password, offline) as service:
        if service.api("subscriptions"):
            raise RuntimeError("Restore target must start with an empty database")
        service.api("backup/restore", "POST", {"name": name})
        verify_subscription(service, custom_path)
    with start(command, target, backups, port, password, offline) as service:
        service.verify_web()
        verify_subscription(service, custom_path)
    print("Passed: embedded web, login, subscription, backup, restore into an empty database, and restart.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--runner", help="Optional emulator executable, for example qemu-arm")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="Ani-Go 平台 ") as directory:
        root = Path(directory)
        executable = root / ("Ani-Go 程序" + args.binary.suffix)
        shutil.copy2(args.binary, executable)
        command = [str(executable)]
        if args.runner:
            command.insert(0, shutil.which(args.runner) or str(Path(args.runner).resolve()))
        # 本地占住但不监听的端口用于外部依赖和出站 HTTP 代理。
        with socket.socket() as blocked, socket.socket() as available:
            blocked.bind((HOST, 0))
            available.bind((HOST, 0))
            port = available.getsockname()[1]
            available.close()
            run(command, root, port, f"http://{HOST}:{blocked.getsockname()[1]}")


if __name__ == "__main__":
    main()
