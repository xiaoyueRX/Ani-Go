# Ani-Go

> Fully automated anime tracking & download manager
> [中文版](README.md)

**Ani-Go** is a Go-based anime manager that auto-tracks via Mikan RSS, completes historical episodes en masse, supports multiple download clients and source sites, and organizes files so they're ready for Jellyfin / fnOS scraping.

## Features

- 📺 **New Season Schedule**: yuc.wiki source, grouped by weekday with poster images
- 🔄 **Auto Track + History Backfill**: subscribe on Mikan and backfill older episodes
- ⬇️ **Multiple Downloaders**: qBittorrent / Transmission / Aria2
- 🗂️ **Auto Organization**: rename + create directories, Jellyfin-ready
- 🌐 **GFW Mirror Fallback**: auto-switching mirrors for Mikan / BGM.tv / TMDB
- 🤖 **AI Assisted** (optional): OpenAI / Google / Anthropic / Ollama
- 🧩 **Plugin System**: open hooks for third-party extensions
- 🌍 **Web UI + PWA**: Vue3 to manage subscriptions, downloads, settings — installable as an app

Fully **bilingual (CN/EN)** with instant language switching.

## Quick Start

```bash
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go
cp .env.example .env        # fill in MIKAN_RSS_URL, QB_HOST, etc.
docker compose up -d
```

Open `http://localhost:20001` in your browser. Default account: `admin` / `admin`.

Manual build:

```bash
cd web && npm install && npm run build && cd ..
go build -o anigo .
./anigo
```

## Environment Variables

See [`.env.example`](.env.example) for the full list. Key ones:

| Variable | Description | Default |
|----------|-------------|---------|
| `MIKAN_RSS_URL` | Mikan personal RSS URL | - |
| `DEFAULT_DOWNLOADER` | Default downloader | `qbittorrent` |
| `QB_HOST` / `QB_USER` / `QB_PASS` | qBittorrent connection | `http://localhost:8081` |
| `TV_BASE_PATH` | Anime root directory | `./TV/番剧` |
| `PORT` | Web UI port | `20001` |

## Current Version

**v0.5.0** — stability hardening & plugin management loop (download-complete notification panic fix, per-subscription stall timeout config, plugin management tab, backup restore fixes).

## License

MIT License © xiaoyueRX
