# Ani-Go

> Fully automated anime tracking and download management system

**Ani-Go** is an open-source anime management tool built with Go, supporting automatic new episode tracking, historical batch completion, multiple download clients, multiple source sites, and file organization compatible with Jellyfin/fnOS.

## Features

- 🔄 **Auto Tracking**: Bind to Mikan personal RSS and automatically track subscriptions from the Mikan web interface
- 📦 **Historical Completion**: Crawl Mikan anime pages to backfill episodes not covered by RSS feeds
- ⬇️ **Multiple Downloaders**: qBittorrent / Transmission / Aria2
- 🗂️ **Auto Organization**: Rename + directory creation, directly recognized by Jellyfin without additional processing
- 🤖 **AI Assisted** (optional): Supports OpenAI / Gemini / Ollama for category recognition
- 🧩 **Plugin System**: Open hooks supporting third-party extensions
- 🌐 **Web UI**: Manage subscriptions, download queues, and settings from a browser
- ⚠️ **Timeout Warning**: Smart dead-link detection with automatic subgroup change suggestions

## Quick Start

```bash
docker run -d \
  -e MIKAN_RSS_URL="https://mikanani.me/RSS/MyBangumi?token=YOUR_TOKEN" \
  -e QB_HOST="http://qbittorrent:8080" \
  -e QB_USER="username" \
  -e QB_PASS="password" \
  -v /your/tv/path:/TV \
  -p 8080:8080 \
  ghcr.io/xiaoyuerx/ani-go:latest
```

## Development

```bash
# Clone the project
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go

# Configure environment variables
cp .env.example .env
# Edit .env and fill in your configuration

# Run
go run .
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MIKAN_RSS_URL` | Mikan personal RSS URL | - |
| `QB_HOST` | qBittorrent address | `http://localhost:8081` |
| `QB_USER` | qBittorrent username | - |
| `QB_PASS` | qBittorrent password | - |
| `DB_PATH` | Database file path | `/data/ani-go.db` |
| `TV_BASE_PATH` | Anime root directory | `/TV/Media/番剧` |
| `PORT` | Web UI port | `8080` |

## License

MIT License © xiaoyueRX
