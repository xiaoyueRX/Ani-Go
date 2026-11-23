"""通过实际 HTTP 更新接口验收本平台的发行包、进程交接和自动回滚。"""

import argparse
from contextlib import contextmanager
from functools import partial
import hashlib
from http.client import HTTPException
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import platform
import secrets
import shutil
import signal
import socket
import subprocess
import sys
import tarfile
import tempfile
import threading
import time
import zipfile

from test_platform_runtime import HOST, TITLE, Service, process_environment, verify_subscription
from probe_armv7_runtime import verify_binary as verify_armv7_binary


FIRST = "v1.0.1"
GOOD = "v1.0.2"
BROKEN = "v1.0.3"
REPO = Path(__file__).resolve().parents[1]
UPDATE = "github.com/xiaoyueRX/Ani-Go/internal/update"


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def build(binary, version, base, env):
    flags = (f"-s -w -X main.version={version} -X {UPDATE}.releaseMetadataURL={base}/latest"
             f" -X {UPDATE}.releaseDownloadBase={base}/download")
    subprocess.run(["go", "build", "-mod=readonly", "-trimpath", "-ldflags", flags, "-o", binary, "."],
                   cwd=REPO, env=env, check=True)


def package(directory, version, target, source):
    name = f"ani-go-{version}-{target}"
    dest = directory / "download" / version
    dest.mkdir(parents=True)
    if os.name == "nt":
        archive = dest / (name + ".zip")
        with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as out:
            out.write(source, name + "/anigo.exe")
    else:
        archive = dest / (name + ".tar.gz")
        with tarfile.open(archive, "w:gz", compresslevel=1) as out:
            out.add(source, arcname=name + "/anigo")
    (dest / "checksums.txt").write_text(f"{sha256(archive)}  {archive.name}\n", encoding="ascii")
    return "/" + archive.relative_to(directory).as_posix()


@contextmanager
def release_server(directory):
    served = set()
    # v2 首次下载后发布 v3，使刚启动的 v2 即使缓存首次检查也能看到坏包。
    good_download = threading.Event()

    class Release(SimpleHTTPRequestHandler):
        def do_GET(self):
            if self.path == "/latest":
                version = BROKEN if good_download.is_set() else GOOD
                body = json.dumps({"tag_name": version}).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
            else:
                super().do_GET()

        def log_request(self, code="-", size="-"):
            if code == 200:
                served.add(self.path)
                if self.path.startswith(f"/download/{GOOD}/ani-go-"):
                    good_download.set()

        def log_message(self, fmt, *args):
            pass

    with ThreadingHTTPServer((HOST, 0), partial(Release, directory=directory)) as server:
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            yield f"http://{HOST}:{server.server_port}", served
        finally:
            server.shutdown()
            thread.join()


def wait_for(label, predicate, log):
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        try:
            if predicate():
                return
        except (OSError, HTTPException):
            pass  # 进程交接期间监听端口会短暂关闭。
        time.sleep(0.15)
    # 日志可能含下载器设置或临时管理凭据，仅显示交接诊断。
    messages = [s for s in log.read_text(encoding="utf-8", errors="replace").splitlines()
                if any(term in s for term in ("[更新管理器]", "更新交接", "已恢复旧版", "已通过健康检查"))]
    raise RuntimeError(f"Timed out waiting for {label}. " + "\n".join(messages[-5:]))


def stop_windows_install(install, psutil):
    # 首次 Windows 交接后初始 PID 已退出，监管进程由改名后的 previous-anigo.exe 运行。
    # 同一安装目录只用于本次测试；用可执行文件路径识别所有权，避免误停同名服务。
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        owned = []
        for proc in psutil.process_iter(["exe"]):
            path = proc.info["exe"]
            if path and Path(path).is_relative_to(install):
                owned.append(proc)
                try:
                    proc.kill()
                except psutil.NoSuchProcess:
                    pass
        if not owned:
            return
        psutil.wait_procs(owned, timeout=1)
    raise RuntimeError("Windows update processes did not stop")


@contextmanager
def running(binary, home, port, password, offline, log):
    if os.name == "nt":
        import psutil  # Windows CI 安装 psutil，以便回收已从初始 PID 脱离的监管进程。
    env = process_environment(home, home / "backups", port, password, offline)
    with log.open("wb") as output:
        proc = subprocess.Popen([binary], cwd=home, env=env, stdin=subprocess.DEVNULL,
                                stdout=output, stderr=subprocess.STDOUT, start_new_session=os.name != "nt")
        try:
            yield Service(port), proc
        finally:
            if os.name == "nt":
                stop_windows_install(binary.parent, psutil)
            else:
                # 即使一次失败的交接留下子进程，也只回收本次创建的会话。
                try:
                    os.killpg(proc.pid, signal.SIGTERM)
                    proc.wait(timeout=10)
                except (ProcessLookupError, subprocess.TimeoutExpired):
                    pass
                finally:
                    try:
                        os.killpg(proc.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
            proc.wait(timeout=15)
    # 测试中原始监管 PID 可能已退出，仍要等同一进程组中的替代服务释放监听端口。
    for _ in range(50):
        with socket.socket() as probe:
            probe.settimeout(0.3)
            if probe.connect_ex((HOST, port)) != 0:
                return
        time.sleep(0.1)
    raise RuntimeError("The update test's HTTP listener stayed open after process cleanup")


def check_late_handoff_failure(service, installed, expected_hash, monitor, log):
    # Linux 对仍有写句柄的映像返回 ETXTBSY，回滚时的 rename 仍可成功。
    held = installed.with_name("held-previous")
    shutil.copy2(installed, held)
    monitor_dir, = installed.parent.glob(".anigo-monitor-*")
    control = monitor_dir / "next.json"

    def have_request():
        try:
            return json.loads(control.read_text(encoding="utf-8")).get("Version") == BROKEN
        except json.JSONDecodeError:
            return False

    with held.open("r+b"):
        service.api("system/update/execute", "POST")
        wait_for("monitor control file", have_request, log)
        previous = Path(json.loads(control.read_text(encoding="utf-8"))["Old"])
        if previous.parent.parent != installed.parent or previous.name != "previous-" + installed.name:
            raise RuntimeError(f"Unexpected rollback path {previous}")
        os.replace(held, previous)
        monitor.kill()
        monitor.wait(timeout=5)
        wait_for("failed handoff", lambda: "text file busy" in log.read_text(encoding="utf-8").lower(), log)
        progress = service.api("system/update/progress")
        if progress.get("status") != "failed" or "text file busy" not in progress.get("error", "").lower():
            raise RuntimeError(f"Failed handoff left unexpected public progress: {progress}")
        if sha256(installed) != expected_hash or service.api("health").get("version") != GOOD:
            raise RuntimeError("Running version or restored executable changed after handoff failure")
    rollbacks = log.read_text(encoding="utf-8").count("已恢复旧版")
    service.api("system/update/execute", "POST")
    wait_for("retry rolls back the invalid release", lambda: log.read_text(encoding="utf-8").count("已恢复旧版") > rollbacks, log)
    wait_for("restored service after retry", lambda: service.api("health").get("version") == GOOD, log)
    if sha256(installed) != expected_hash:
        raise RuntimeError("Retry did not restore the verified executable")
    print("Passed Linux handoff failure: public failed status, verified rollback image and accepted retry.")


def exercise(service, installed, expected_hash, home, offline, password, log, monitor):
    def healthy(version):
        reply = service.api("health")
        return reply.get("version") == version and reply.get("status") == "ok"

    wait_for("first version", lambda: healthy(FIRST), log)
    service.login(password)
    custom = home / "追番目录"
    custom.mkdir()
    row = service.api("subscriptions", "POST", {"title_cn": TITLE, "season": 2, "custom_path": str(custom),
                                               "rss_url": offline + "/rss"}, status=201)
    service.api(f"subscriptions/{row['id']}", "PUT", {"enabled": False, "total_episodes": 7})

    for before, target, marker in [
        (FIRST, GOOD, f"新版 {GOOD} 已通过健康检查"),
        (GOOD, BROKEN, "新版未通过健康检查，已恢复旧版"),
    ]:
        info = service.api("system/update/check")
        if (info.get("current_version"), info.get("latest_version"), info.get("has_update"),
                info.get("can_self_update")) != (before, target, True, True):
            raise RuntimeError(f"Unexpected update capability or release during {before} -> {target}: {info}")
        service.api("system/update/execute", "POST")
        # 回滚后仍是 v2；先等监管者确认已恢复旧文件，再检查健康，避免把交接前的 v2 认成成功。
        wait_for(marker, lambda: marker in log.read_text(encoding="utf-8", errors="replace") and healthy(GOOD), log)
        if sha256(installed) != expected_hash:
            raise RuntimeError(f"Installed binary after {target} differs from the verified {GOOD} image")
        verify_subscription(service, custom)
        row_path = f"subscriptions/{row['id']}"
        Service(service.port).request("/api/" + row_path, status=401)
        if service.api(row_path)["subscription"]["title_cn"] != TITLE:
            raise RuntimeError("Original subscription ID or authenticated session was lost")
    if sys.platform.startswith("linux"):
        check_late_handoff_failure(service, installed, expected_hash, monitor, log)
        verify_subscription(service, custom)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--armv7", action="store_true", help="在支持直接执行 ARMv7 的 Linux ARM64 主机上验收 32 位发行包")
    args = parser.parse_args()
    # 原生 CI 已准备好 Go 工具链、Go 模块和 web/dist；模拟发行包与测试进程均只使用回环网络。
    env = dict(os.environ, CGO_ENABLED="0", GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local")
    host, arch = subprocess.check_output(["go", "env", "GOHOSTOS", "GOHOSTARCH"], cwd=REPO, env=env, text=True).split()
    if args.armv7:
        if (host, arch) != ("linux", "arm64") or sys.platform != "linux" or platform.machine().lower() not in ("aarch64", "arm64"):
            parser.error("--armv7 requires a Linux ARM64 host and native ARM64 Go toolchain")
        arch = "arm"
    if host == "linux" and arch == "arm":
        env["GOARM"] = "7"
    env.update(GOOS=host, GOARCH=arch)
    target = host + "-" + ("armv7" if arch == "arm" else arch)
    with tempfile.TemporaryDirectory(prefix="Ani-Go 原生更新 ") as folder, socket.socket() as blocked:
        root = Path(folder).resolve()
        home = root / "安装目录"
        home.mkdir()
        releases = root / "发行"
        releases.mkdir()
        installed = home / ("anigo.exe" if os.name == "nt" else "anigo")
        blocked.bind((HOST, 0))
        offline = f"http://{HOST}:{blocked.getsockname()[1]}"
        with release_server(releases) as (base, served):
            build(installed, FIRST, base, env)
            good = root / ("good" + installed.suffix)
            build(good, GOOD, base, env)
            if args.armv7:
                for binary in (installed, good):
                    verify_armv7_binary(binary)
                print(f"Verified {FIRST} and {GOOD}: static ELF32 ARM, GOOS=linux GOARCH=arm GOARM=7.", flush=True)
            wanted_hash = sha256(good)
            broken = root / "invalid-program"
            broken.write_bytes(b"synthetic release: not an executable\n")
            packages = {package(releases, GOOD, target, good), package(releases, BROKEN, target, broken)}
            with socket.socket() as free:
                free.bind((HOST, 0))
                port = free.getsockname()[1]
            password = secrets.token_urlsafe(32)
            log = root / "app.log"
            with running(installed, home, port, password, offline, log) as (service, monitor):
                if args.armv7 and not Path(f"/proc/{monitor.pid}/exe").samefile(installed):
                    raise RuntimeError("The running process is not the verified ARMv7 executable")
                exercise(service, installed, wanted_hash, home, offline, password, log, monitor)
            expected = packages | {"/latest", f"/download/{GOOD}/checksums.txt", f"/download/{BROKEN}/checksums.txt"}
            if not expected.issubset(served):
                raise RuntimeError(f"Missing local release requests: {sorted(expected - served)}")
        print(f"Passed {target}: HTTP upgrade {FIRST} -> {GOOD}, rollback from {BROKEN}, "
              "binary SHA-256, original port, login session and subscription.")


if __name__ == "__main__":
    main()
