"""用发布 Dockerfile 的原生 Linux 镜像验证首次登录、网页和跨容器卷持久化。"""

import argparse
from contextlib import contextmanager
from http.client import HTTPException
import json
import os
import platform
import secrets
import subprocess
import time

from test_platform_runtime import Service, TITLE, verify_subscription


DB_PATH = "/data/db/ani-go.db"
DOWNLOADS = "/data/downloads"
MACHINES = {"amd64": "x86_64", "arm64": "aarch64"}
ELF_MACHINES = {"amd64": 62, "arm64": 183}


def docker(*args, env=None):
    return subprocess.check_output(["docker", *args], text=True, env=env).strip()


def verify_executable(image, arch):
    header = bytes.fromhex(docker("run", "--rm", "--network", "none", "--entrypoint", "od",
                                  image, "-An", "-tx1", "-N20", "/app/anigo"))
    if len(header) != 20 or header[:6] != b"\x7fELF\x02\x01":
        raise RuntimeError("Expected a 64-bit little-endian Linux ELF at /app/anigo")
    machine = int.from_bytes(header[18:20], "little")
    if machine != ELF_MACHINES[arch]:
        raise RuntimeError(f"Image {arch} contains /app/anigo ELF machine {machine}, expected {ELF_MACHINES[arch]}")
    print(f"Verified /app/anigo ELF machine {machine} for linux/{arch}", flush=True)


@contextmanager
def running(image, volume, password):
    # 密码通过进程环境传给 Docker，命令和异常输出均不含其值。
    cid = docker("run", "-d", "-p", "127.0.0.1::20001", "--mount", f"type=volume,src={volume},dst=/data",
                 "--health-interval=1s", "--health-start-period=1s",
                 "-e", "ANIGO_ADMIN_PASSWORD", "-e", f"DB_PATH={DB_PATH}", "-e", "RSS_INTERVAL=24h",
                 "-e", "MIKAN_DOMAIN=http://127.0.0.1:9", "-e", "MIKAN_RSS_URL=http://127.0.0.1:9/rss",
                 "-e", "HTTP_PROXY=http://127.0.0.1:9", "-e", "HTTPS_PROXY=http://127.0.0.1:9",
                 # Docker 客户端可以从本机配置注入小写代理。BusyBox wget 的回环健康检查应直接连接。
                 "-e", "http_proxy=", "-e", "https_proxy=", "-e", "all_proxy=", "-e", "NO_PROXY=127.0.0.1",
                 image, env={**os.environ, "ANIGO_ADMIN_PASSWORD": password})
    try:
        mounts = json.loads(docker("inspect", "--format", "{{json .Mounts}}", cid))
        if not any(m["Type"] == "volume" and m["Name"] == volume and m["Destination"] == "/data" and m["RW"] for m in mounts):
            raise RuntimeError("Container must mount the test volume read-write at /data")
        address = docker("port", cid, "20001/tcp")
        host, port = address.rsplit(":", 1)
        if host != "127.0.0.1":
            raise RuntimeError("Test port must be loopback-only")
        service = Service(int(port))
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            try:
                health = service.api("health")
            except (OSError, HTTPException):
                if docker("inspect", "--format", "{{.State.Running}}", cid) != "true":
                    raise RuntimeError("Container exited before its health endpoint was ready")
                time.sleep(0.25)
                continue
            if health.get("status") != "ok" or not health.get("version"):
                raise RuntimeError("Container health response lacks status or version")
            state = docker("inspect", "--format", "{{.State.Health.Status}}", cid)
            if state == "healthy":
                break
            if state != "starting":
                raise RuntimeError(f"Image health check reported {state}")
            time.sleep(0.25)
        else:
            raise RuntimeError("Container and HTTP endpoint did not become healthy")
        service.request("/api/subscriptions", status=401)
        if b'id="app"' not in service.request("/")[1]:
            raise RuntimeError("Embedded UI is missing its app element")
        service.verify_web()
        yield cid, service
        docker("stop", "--timeout", "15", cid)
    finally:
        docker("rm", "-f", cid)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("image")
    parser.add_argument("--arch", required=True, choices=MACHINES)
    args = parser.parse_args()
    machine = platform.machine()
    image_platform = docker("image", "inspect", "--format", "{{.Os}}/{{.Architecture}}", args.image)
    if platform.system() != "Linux" or machine != MACHINES[args.arch] or image_platform != "linux/" + args.arch:
        raise RuntimeError(f"Expected native linux/{args.arch}, got host {platform.system()}/{machine}, image {image_platform}")
    verify_executable(args.image, args.arch)
    password = secrets.token_urlsafe(24)
    volume = docker("volume", "create")
    try:
        with running(args.image, volume, password) as (first, service):
            container_machine = docker("exec", first, "uname", "-m")
            if container_machine != machine:
                raise RuntimeError("Container architecture does not match the native runner")
            identity = docker("exec", first, "sh", "-c", 'echo "$(id -u):$(id -g)"')
            if identity != "1000:1000" or docker("exec", first, "stat", "-c", "%u:%g", DB_PATH) != identity:
                raise RuntimeError("Application and volume database must use UID:GID 1000:1000")
            service.login(password)
            if service.api("subscriptions") != []:
                raise RuntimeError("Initial volume must have no subscriptions")
            sub = service.api("subscriptions", "POST", {"title_cn": TITLE, "season": 2, "custom_path": DOWNLOADS,
                                                        "rss_url": "http://127.0.0.1:9/rss"}, status=201)
            service.api(f"subscriptions/{sub['id']}", "PUT", {"enabled": False, "total_episodes": 7})
        with running(args.image, volume, secrets.token_urlsafe(24)) as (second, service):
            if second == first:
                raise RuntimeError("Expected a newly created container")
            service.login(password)
            verify_subscription(service, DOWNLOADS)
            if service.api("subscriptions")[0]["id"] != sub["id"]:
                raise RuntimeError("Subscription ID changed when the container was recreated")
    finally:
        docker("volume", "rm", volume)
    print(f"ANIGO_CONTAINER_CHECK host={machine} image={image_platform} container={container_machine} uid_gid={identity} "
          f"db={DB_PATH} health=ok docker_health=healthy ui_module=ok login=ok recreated=ok subscription_id={sub['id']}", flush=True)


if __name__ == "__main__":
    main()
