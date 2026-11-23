"""Check Markdown links, examples and the API contract from a source checkout."""

import json
import os
import re
import subprocess
import sys
from pathlib import Path
from urllib.parse import unquote, urlsplit

import yaml
from markdown_it import MarkdownIt
from openapi_spec_validator import validate

ROOT = Path(__file__).resolve().parents[1]
MARKDOWN = MarkdownIt("commonmark").enable(["table", "strikethrough"])
HTTP_METHODS = {"get", "post", "put", "patch", "delete", "head", "options", "trace"}


def read(path):
    return path.read_text(encoding="utf-8")


def headings_and_links(tokens):
    anchors, links = set(), []
    heading = False
    for token in tokens:
        if token.type == "heading_open":
            heading = True
        elif token.type == "heading_close":
            heading = False
        elif token.type == "inline":
            text = ""
            for child in token.children or []:
                if child.type == "link_open":
                    links.append(child.attrGet("href"))
                elif child.type == "image":
                    links.append(child.attrGet("src"))
                elif child.type in ("text", "code_inline"):
                    text += child.content
            if heading:
                slug = re.sub(r"[^\w\- ]", "", text.lower()).replace(" ", "-")
                anchor, suffix = slug, 0
                while anchor in anchors:
                    suffix += 1
                    anchor = f"{slug}-{suffix}"
                anchors.add(anchor)
    return anchors, links


def check_markdown(root):
    anchors, links, examples = {}, [], {}
    for directory, subdirs, names in os.walk(root):
        subdirs[:] = [name for name in subdirs if name not in {
            ".git", ".gitnexus", ".venv", "node_modules", "dist", "__pycache__",
        }]
        for name in sorted(names):
            if not name.endswith(".md"):
                continue
            path = Path(directory, name)
            # Terminate the final content line so fence source spans are comparable at EOF.
            tokens = MARKDOWN.parse(read(path) + "\n")
            anchors[path], targets = headings_and_links(tokens)
            links.extend((path, target) for target in targets)
            for token in tokens:
                if token.type != "fence":
                    continue
                start, end = token.map
                # CommonMark can close a fence implicitly at EOF or at the end of a list or quote.
                if end - start != token.content.count("\n") + 2:
                    raise ValueError(f"{path.relative_to(root)}:{start + 1}: unclosed code fence")
                language = token.info.strip().split()
                if language[:1] == ["json"]:
                    try:
                        examples.setdefault(path, []).append(json.loads(token.content))
                    except ValueError as error:
                        raise ValueError(f"{path.relative_to(root)}:{token.map[0] + 1}: {error}") from error
                elif language[:1] in (["shell"], ["bash"], ["sh"]):
                    result = subprocess.run(["bash", "-n"], input=token.content, text=True, capture_output=True)
                    if result.returncode:
                        raise ValueError(f"{path.relative_to(root)}:{token.map[0] + 1}: {result.stderr.strip()}")

    for source, target in links:
        url = urlsplit(target)
        if url.scheme or url.netloc:
            continue
        path = unquote(url.path)
        if path.startswith("/"):
            destination = (root / path.lstrip("/")).resolve()
        else:
            destination = (source.parent / path).resolve() if path else source
        if not destination.is_relative_to(root):
            raise ValueError(f"{source.relative_to(root)}: link target outside checkout {target}")
        if not destination.exists():
            raise ValueError(f"{source.relative_to(root)}: missing link target {target}")
        if url.fragment and destination in anchors and unquote(url.fragment) not in anchors[destination]:
            raise ValueError(f"{source.relative_to(root)}: missing heading {target}")

    zh = examples[root / "docs/02_PLUGIN_SPEC.md"]
    en = examples[root / "docs/02_PLUGIN_SPEC_EN.md"]
    manifest = json.loads(read(root / "examples/plugins/cloud-tasks.json"))
    if not zh or zh != en or zh[0] != manifest:
        raise ValueError("The two plugin guides and importable manifest disagree")
    return len(anchors)


def registered_routes(root):
    source = read(root / "internal/api/server.go").split("func (s *Server) registerRoutes(", 1)[1]
    source = source.split("\n}", 1)[0]
    routes = set()
    # Go's two methodless registrations implement these public endpoints.
    public = {"/api/login": "POST", "/api/health": "GET"}
    methods = []
    for line in source.splitlines():
        loop = re.match(r'\s*for _, method := range \[\]string\{([^}]+)\} \{', line)
        if loop:
            methods = re.findall(r'"([A-Z]+)"', loop[1])
        elif line.strip() == "}":
            methods = []
        match = re.match(r'\s*mux\.Handle(?:Func)?\(([^,]+),', line)
        if not match:
            continue
        arg = match[1].strip()
        if re.fullmatch(r'method\s*\+\s*" /[^"]+"', arg):
            path = arg.split('"')[1].strip()
            if not methods:
                raise ValueError(f"Cannot resolve loop registering {path}")
            routes.update((m, path) for m in methods)
        elif re.fullmatch(r'"[A-Z]+ /[^"]+"', arg):
            method, path = arg.strip('"').split(" ", 1)
            # The handler reads the ID following this legacy prefix.
            if path == "/api/bangumi/subject/":
                path += "{id}"
            routes.add((method, path))
        elif arg.strip('"') in public:
            path = arg.strip('"')
            routes.add((public[path], path))
        else:
            raise ValueError(f"Cannot resolve Go route {arg}")
    if not routes:
        raise ValueError("No Go routes found in registerRoutes")
    return routes


def check_api(root):
    spec = yaml.safe_load(read(root / "docs/api.yaml"))
    validate(spec)
    expected = registered_routes(root)
    documented = set()
    for path, item in spec["paths"].items():
        for method, operation in item.items():
            if method in HTTP_METHODS:
                servers = operation.get("servers", item.get("servers", spec["servers"]))
                documented.add((method.upper(), servers[0]["url"].rstrip("/") + path))
    table = set()
    for line in read(root / "docs/API.md").splitlines():
        if line.startswith("|"):
            cells = line.split("|")
            if len(cells) < 4:
                continue
            for method in re.findall(r"`([A-Z]+)`", cells[1]):
                for path in re.findall(r"`(/(?:api|mcp)/[^`]+)`", cells[2]):
                    table.add((method, path))
    for name, routes in (("docs/api.yaml", documented), ("docs/API.md", table)):
        if routes != expected:
            raise ValueError(f"{name}: missing {sorted(expected - routes)}, extra {sorted(routes - expected)}")
    return len(expected)


if __name__ == "__main__":
    try:
        count = check_markdown(ROOT)
        routes = check_api(ROOT)
    except (KeyError, ValueError, OSError) as error:
        sys.exit(str(error))
    print(f"Validated {count} Markdown files, plugin examples and {routes} API routes.")
