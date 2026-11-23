"""用本机 HTTP 提供合成 RSS、种子与分段下载，校验真实下载器整理后的媒体内容。"""

from contextlib import contextmanager
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import re
import socket
from threading import Thread
import time
from urllib.parse import unquote, urlsplit
from xml.sax.saxutils import escape

from test_platform_runtime import HOST


PIECE_SIZE = 16 * 1024
TITLE = "Harbor Archive"
NAMES = [f"{TITLE} S01E{episode:02}.mkv" for episode in (1, 2, 3)]
PAYLOADS = [hashlib.shake_256(name.encode()).digest(size)
            for name, size in zip(NAMES, (203_111, 181_337, 223_919))]


def bencode(value):
    if isinstance(value, int):
        return b"i" + str(value).encode() + b"e"
    if isinstance(value, bytes):
        return str(len(value)).encode() + b":" + value
    if isinstance(value, list):
        return b"l" + b"".join(bencode(item) for item in value) + b"e"
    return b"d" + b"".join(bencode(key) + bencode(value[key]) for key in sorted(value)) + b"e"


class SeedHandler(BaseHTTPRequestHandler):
    def handle(self):
        try:
            super().handle()
        except ConnectionError:
            # 客户端暂停或退出时可以提前关闭正在传输的分段请求。
            pass

    def log_message(self, *args):
        pass

    def do_GET(self):
        path = unquote(urlsplit(self.path).path)
        if path.startswith("/RSS/"):
            data, mime = self.server.rss, "application/rss+xml"
        elif path == "/pack.torrent":
            data, mime = self.server.torrent, "application/x-bittorrent"
        elif path in self.server.media:
            if self.server.media_ready is not None and not self.server.media_ready.wait(timeout=60):
                self.send_error(503)
                return
            data = self.server.media[path]
            mime = "application/x-bittorrent" if path.endswith(".torrent") else "application/octet-stream"
        else:
            self.send_error(404)
            return

        start_byte, end_byte = 0, len(data) - 1
        requested = self.headers.get("Range")
        if requested:
            match = re.fullmatch(r"bytes=(\d+)-(\d*)", requested)
            if not match:
                self.send_error(416)
                return
            start_byte = int(match[1])
            if match[2]:
                end_byte = min(end_byte, int(match[2]))
            if start_byte > end_byte:
                self.send_error(416)
                return
        self.send_response(206 if requested else 200)
        if requested:
            self.send_header("Content-Range", f"bytes {start_byte}-{end_byte}/{len(data)}")
        self.send_header("Content-Type", mime)
        self.send_header("Accept-Ranges", "bytes")
        self.send_header("Content-Length", str(end_byte - start_byte + 1))
        self.end_headers()
        self.wfile.write(data[start_byte:end_byte + 1])


@contextmanager
def web_seed(media_ready=None, *, http_seed=True, payloads=PAYLOADS):
    piece = PIECE_SIZE
    content = b"".join(payloads)
    files = [{b"length": len(data), b"path": [name.encode()]} for name, data in zip(NAMES, payloads)]
    info = {b"name": TITLE.encode(), b"files": files, b"piece length": piece, b"private": 1,
            b"pieces": b"".join(hashlib.sha1(content[i:i+piece]).digest() for i in range(0, len(content), piece))}
    infohash = hashlib.sha1(bencode(info)).hexdigest()
    with ThreadingHTTPServer((HOST, 0), SeedHandler) as server:
        url = f"http://{HOST}:{server.server_port}"
        torrent = {b"info": info}
        if http_seed:
            torrent[b"url-list"] = [(url + "/media/").encode()]
        server.torrent = bencode(torrent)
        server.media_ready = media_ready
        server.media = {f"/media/{TITLE}/{name}": data for name, data in zip(NAMES, payloads)} if http_seed else {}
        server.rss = f"""<rss version="2.0"><channel><title>{TITLE}</title><item>
        <title>[Fixture] {TITLE} [01-03] [1080p]</title><link>{url}/Home/Episode/{infohash}</link>
        <enclosure url="{url}/pack.torrent" length="{len(server.torrent)}" type="application/x-bittorrent"/>
        </item></channel></rss>""".encode()
        worker = Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            yield url, infohash
        finally:
            if media_ready is not None:
                media_ready.set()
            server.shutdown()
            worker.join()


@contextmanager
def movie_feed(title):
    """逐条发布私有单文件种子，同一份回环 RSS 保留先前发行。"""
    with ThreadingHTTPServer((HOST, 0), SeedHandler) as server:
        url = f"http://{HOST}:{server.server_port}"
        server.media, server.media_ready = {}, None
        items = []

        def rss():
            return f'<rss version="2.0"><channel><title>{escape(title)}</title>{"".join(items)}</channel></rss>'.encode()

        def publish(release, payload):
            info = {b"name": (release + ".mkv").encode(), b"length": len(payload), b"private": 1,
                    b"piece length": PIECE_SIZE,
                    b"pieces": b"".join(hashlib.sha1(payload[i:i + PIECE_SIZE]).digest()
                                        for i in range(0, len(payload), PIECE_SIZE))}
            infohash = hashlib.sha1(bencode(info)).hexdigest()
            path = f"/{infohash}.torrent"
            torrent = bencode({b"info": info})
            server.media[path] = torrent
            items.append(f'<item><title>{escape(release)}</title><link>{url}/Home/Episode/{infohash}</link>'
                         f'<enclosure url="{url}{path}" length="{len(torrent)}" type="application/x-bittorrent"/></item>')
            server.rss = rss()
            return infohash, path

        server.rss = rss()
        worker = Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            yield url, publish
        finally:
            server.shutdown()
            worker.join()


def free_port():
    with socket.socket() as available:
        available.bind((HOST, 0))
        return available.getsockname()[1]


def wait_for(description, fetch, ready, timeout=90):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = fetch()
        if ready(value):
            return value
        time.sleep(0.25)
    raise RuntimeError("Timed out waiting for " + description)


def create_pack_subscription(service, url, downloads):
    subscription = service.api("subscriptions", "POST", {
        "title_cn": TITLE, "season": 1, "mikan_id": "42", "bangumi_id": "42",
        "rss_url": url + "/RSS/Bangumi?bangumiId=42", "custom_path": str(downloads),
    }, status=201)
    route = f"subscriptions/{subscription['id']}"
    service.api(route, "PUT", {"total_episodes": 3})
    return route


def verify_library(service, route, home, downloads, payloads=PAYLOADS):
    detail = wait_for("selected episodes to be organized", lambda: service.api(route),
                      lambda data: sum(row["status"] == "organized" for row in data["episodes"]) >= 2)
    rows = sorted(detail["episodes"], key=lambda row: row["number"])
    if [row["number"] for row in rows] != [1, 3] or detail["subscription"]["current_episodes"] != 2:
        raise RuntimeError("The library subscription contains unexpected episodes")
    library = (home / "剧集").resolve()
    media = list(library.rglob("*.mkv"))
    if any(file.is_symlink() for file in media):
        raise RuntimeError("The library contains a symbolic link instead of a media file")
    targets = {file.resolve() for file in media}
    if len(targets) != 2:
        raise RuntimeError("Expected two media files in the library")
    for row in rows:
        i = int(row["number"]) - 1
        target = Path(row["final_path"]).resolve()
        source = downloads / TITLE / NAMES[i]
        if not target.is_relative_to(library) or target not in targets or row["file_size"] != len(payloads[i]):
            raise RuntimeError(f"Incorrect library path or size for episode {i+1}")
        if source.read_bytes() != payloads[i] or target.read_bytes() != payloads[i]:
            raise RuntimeError(f"Web Seed, download and library differ for episode {i+1}")
