# Ani-Go

> Fully automated anime tracking and download management system

**Ani-Go** is an open-source anime management tool built with Go, supporting automatic new episode tracking, historical batch completion, multiple download clients, multiple source sites, and file organization compatible with Jellyfin/fnOS.

## Features

- 🔄 **Auto Tracking**: Bind to Mikan personal RSS and automatically track subscriptions from the Mikan web interface
- 📦 **Historical Completion**: Crawl Mikan anime pages to backfill episodes not covered by RSS feeds
- ⬇️ **Multiple Downloaders**: qBittorrent / Transmission / Aria2
- 🗂️ **Auto Organization**: Rename + directory creation, directly recognized by Jellyfin without additional processing
- 🌐 **GFW Mirror Fallback**: Multi-domain mirror auto-switching for Mikan / BGM.tv / TMDB
- 🤖 **AI Assisted** (optional): Supports OpenAI / Gemini / Ollama for category recognition
- 🧩 **Plugin System**: Open hooks supporting third-party extensions
- 🌐 **Web UI**: Vue3 + DaisyUI, manage subscriptions, download queues, and settings from a browser
- 🔐 **JWT Auth**: Dynamic secret + Bcrypt, secure and reliable
- 📡 **RESTful API**: Complete subscription CRUD, download management, and settings endpoints

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

# Install frontend dependencies (first time)
cd web && npm install && cd ..

# Build and run
go run .
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MIKAN_RSS_URL` | Mikan personal RSS URL | - |
| `MIKAN_DOMAIN` | Mikan primary domain | `mikanani.me` |
| `MIKAN_PROXY_DOMAIN` | Mikan proxy domain (GFW) | - |
| `MIKAN_MIRROR_DOMAINS` | Mikan mirror domains (comma-separated) | `mikanani.me,mikanime.tv` |
| `QB_HOST` | qBittorrent address | `http://localhost:8081` |
| `QB_USER` | qBittorrent username | - |
| `QB_PASS` | qBittorrent password | - |
| `TMDB_API_KEY` | TMDB API Key | - |
| `TMDB_MIRROR_DOMAINS` | TMDB mirror domains | - |
| `BGMTV_USER_TOKEN` | BGM.tv user token | - |
| `BGMTV_MIRROR_DOMAINS` | BGM mirror domains | `api.bgm.tv,api.bangumi.tv,api.chii.in` |
| `DB_PATH` | Database file path | `/data/ani-go.db` |
| `TV_BASE_PATH` | Anime root directory | `/TV/Media/番剧` |
| `MOVIE_BASE_PATH` | Movie root directory | `/TV/Media/剧场版` |
| `OVA_BASE_PATH` | OVA root directory | - |
| `PORT` | Web UI port | `8080` |

## Project Structure

```
Ani-Go/
├── main.go                  # Entry point
├── internal/
│   ├── api/                 # HTTP API (routes, middleware, handlers)
│   ├── auth/                # JWT auth + Bcrypt
│   ├── config/              # Config loading (env vars first)
│   ├── core/                # Core interfaces & type definitions
│   ├── database/            # GORM models + DB initialization
│   ├── downloader/          # Downloader implementations (qBittorrent)
│   ├── event/               # EventBus
│   ├── metadata/            # Metadata providers (TMDB, BGM.tv)
│   ├── organizer/           # File organizer
│   ├── scheduler/           # Scheduled task runner
│   └── source/              # Source site implementations (Mikan RSS + HTML crawling)
├── web/                     # Vue3 frontend
├── docs/                    # Documentation (bilingual CN/EN)
├── .env.example             # Environment variable template
└── CLAUDE.md                # Claude Code guidance
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/login` | Login to get JWT token |
| `GET` | `/api/me` | Get current user info |
| `GET` | `/api/health` | Health check |
| `GET` | `/api/subscriptions` | List subscriptions |
| `POST` | `/api/subscriptions` | Create subscription |
| `GET` | `/api/subscriptions/{id}` | Subscription detail + episodes |
| `PUT` | `/api/subscriptions/{id}` | Update subscription |
| `DELETE` | `/api/subscriptions/{id}` | Delete subscription |
| `POST` | `/api/subscriptions/{id}/trigger-supplement` | Trigger history backfill |
| `GET` | `/api/downloads` | Download queue |
| `GET` | `/api/settings` | Get settings |
| `PUT` | `/api/settings` | Update settings |

## Development Progress

| Phase | Status |
|-------|--------|
| Phase 0: Project Initialization | ✅ Complete |
| Phase 1: Core Engine MVP | ✅ Complete |
| Phase 2: Historical Completion + Metadata | ✅ Complete |
| Phase 3: Web UI + RESTful API | 🚧 In Progress |
| Phase 4: AI + Multi-Downloader + Plugins | 📅 Planned |
| Phase 5: Multi-Platform Messaging | 📅 Planned |

## License

MIT License © xiaoyueRX
