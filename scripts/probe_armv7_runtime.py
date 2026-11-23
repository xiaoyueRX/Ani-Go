"""探测 Linux runner 能否由内核直接执行静态 ARMv7 Ani-Go，并在支持时运行平台验收。"""

import argparse
import errno
import os
from pathlib import Path
import platform
import re
import secrets
import socket
import subprocess
import sys
import tempfile

from test_platform_runtime import HOST, process_environment


def verify_binary(binary):
    report = subprocess.check_output(["readelf", "-W", "-h", "-l", binary], env={**os.environ, "LC_ALL": "C"}, text=True)
    if not re.search(r"^\s*Class:\s+ELF32\s*$", report, re.M) or not re.search(r"^\s*Machine:\s+ARM\s*$", report, re.M):
        raise ValueError("Expected an ELF32 ARM executable")
    if re.search(r"^\s*(INTERP|DYNAMIC)\s", report, re.M):
        raise ValueError("Expected a statically linked executable without a program interpreter")
    info = subprocess.check_output(["go", "version", "-m", binary], text=True)
    fields = {line.strip() for line in info.splitlines()}
    if not {"path\tgithub.com/xiaoyueRX/Ani-Go", "build\tGOOS=linux", "build\tGOARCH=arm"}.issubset(fields) or not any(
        re.fullmatch(r"build\tGOARM=7(?:,(?:hardfloat|softfloat))?", field) for field in fields
    ):
        raise ValueError("Expected Ani-Go built with GOOS=linux GOARCH=arm GOARM=7")


def executes_directly(binary):
    with tempfile.TemporaryDirectory(prefix="Ani-Go ARMv7 ") as directory, socket.socket() as blocked:
        home = Path(directory)
        blocked.bind((HOST, 0))
        with socket.socket() as available:
            available.bind((HOST, 0))
            port = available.getsockname()[1]
        offline = f"http://{HOST}:{blocked.getsockname()[1]}"
        env = process_environment(home, home / "backups", port, secrets.token_urlsafe(32), offline)
        try:
            proc = subprocess.Popen([binary], cwd=home, env=env, stdin=subprocess.DEVNULL,
                                    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        except OSError as exc:
            if exc.errno != errno.ENOEXEC:
                raise
            print(f"ARMv7 unavailable: kernel on {platform.machine()} rejected direct exec (ENOEXEC).")
            return False
        try:
            image = Path(f"/proc/{proc.pid}/exe")
            if not image.samefile(binary):
                print(f"ARMv7 interpreted: /proc/{proc.pid}/exe is {image.readlink()}; native coverage unavailable.")
                return False
            identity = image.stat()
            print(f"Direct ARMv7 exec on {platform.machine()}: /proc/{proc.pid}/exe matches the ELF32 ARM file "
                  f"(device {identity.st_dev}, inode {identity.st_ino}).", flush=True)
            return True
        finally:
            if proc.poll() is None:
                proc.terminate()
                try:
                    proc.wait(timeout=20)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait()
                    raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--downloaders", action="store_true", help="用本机真实的 aria2、Transmission 和 qBittorrent 验收 ARMv7 应用")
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    verify_binary(binary)
    print(f"Verified {binary.name}: static ELF32 ARM, Ani-Go linux/arm GOARM=7.", flush=True)
    if not executes_directly(binary):
        return
    scripts = Path(__file__).resolve().parent
    subprocess.run([sys.executable, scripts / "test_platform_runtime.py", binary], check=True)
    subprocess.run([sys.executable, scripts / "test_update_runtime.py", "--armv7"], check=True)
    if args.downloaders:
        for script in ("test_aria2_runtime.py", "test_transmission_runtime.py", "test_qb_runtime.py"):
            print(f"Running {script} against {binary.name} (ARMv7).", flush=True)
            subprocess.run([sys.executable, scripts / script, binary], check=True)
    env = dict(os.environ, CGO_ENABLED="0", GOOS="linux", GOARCH="arm", GOARM="7")
    # 直接由内核执行每个测试程序，避免 Go 在 PATH 中自动选择跨架构模拟器。
    subprocess.run(["go", "test", "-exec", "/usr/bin/env", "-count=1", "-timeout=5m", "./..."],
                   cwd=scripts.parent, env=env, check=True)
    print("Passed: direct ARMv7 platform acceptance, self-update and rollback, and native Go package tests.")


if __name__ == "__main__":
    main()
