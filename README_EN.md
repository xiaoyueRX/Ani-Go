# Ani-Go

> Fully automated anime tracking and download management system
> [中文版](README.md)

**Ani-Go** is an open-source anime management tool built with Go, supporting automatic new episode tracking, historical batch completion, multiple download clients, multiple source sites, and file organization compatible with Jellyfin/fnOS.

## Features

- 📺 **New Season Schedule**: yuc.wiki data source, weekly grouping with poster images, auto-refresh every 30 min
- 🔄 **Auto Tracking**: Bind to Mikan personal RSS and automatically track subscriptions from the Mikan web interface
- 📦 **Historical Completion**: Crawl Mikan anime pages to backfill episodes not covered by RSS feeds
- ⬇️ **Multiple Downloaders**: qBittorrent / Transmission / Aria2
- 🗂️ **Auto Organization**: Rename + directory creation, directly recognized by Jellyfin without additional processing
- 🌐 **GFW Mirror Fallback**: Multi-domain mirror auto-switching for Mikan / BGM.tv / TMDB
- 🤖 **AI Assisted** (optional): Supports OpenAI / Google / Anthropic / Ollama 4 protocols for category recognition
- 🧩 **Plugin System**: Open hooks supporting third-party extensions
- 🌐 **Web UI**: Vue3 + DaisyUI, manage subscriptions, download queues, and settings from a browser
- 🔐 **JWT Auth**: Dynamic secret + Bcrypt, secure and reliable
- 📡 **RESTful API**: Complete subscription CRUD, download management, and settings endpoints

## Quick Start

### Docker Compose (Recommended)

```bash
# Clone the project
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go

# Configure environment variables
cp .env.example .env
# Edit .env and fill in your MIKAN_RSS_URL, QB_HOST, etc.

# One-click start
docker compose up -d
```

Open `http://localhost:20001` in your browser. Default account: `admin` / `admin`.

### Docker Single Container

```bash
docker run -d \
  --name ani-go \
  -e MIKAN_RSS_URL="https://mikanani.me/RSS/MyBangumi?token=YOUR_TOKEN" \
  -e QB_HOST="http://qbittorrent:8080" \
  -e QB_USER="username" \
  -e QB_PASS="password" \
  -v /your/tv/path:/TV \
  -p 20001:20001 \
  ghcr.io/xiaoyuerx/ani-go:latest
```

### Manual Build

```bash
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go

# Build frontend
cd web && npm install && npm run build && cd ..

# Build and run (frontend embedded in binary)
go build -o anigo .
./anigo
```

## Development

```bash
# Clone the project
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go

# Configure environment variables
cp .env.example .env

# Frontend dev (Vite HMR)
cd web && npm install && npm run dev

# Backend dev
go run .
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MIKAN_RSS_URL` | Mikan personal RSS URL | - |
| `MIKAN_DOMAIN` | Mikan primary domain | `mikanani.me` |
| `MIKAN_PROXY_DOMAIN` | Mikan proxy domain (GFW) | - |
| `MIKAN_MIRROR_DOMAINS` | Mikan mirror domains (comma-separated) | `mikanani.me,mikanime.tv` |
| `DEFAULT_DOWNLOADER` | Default downloader | `qbittorrent` |
| `QB_HOST` | qBittorrent address | `http://localhost:8081` |
| `QB_USER` | qBittorrent username | - |
| `QB_PASS` | qBittorrent password | - |
| `TR_HOST` | Transmission address | `http://localhost:9091` |
| `TR_USER` | Transmission username | - |
| `TR_PASS` | Transmission password | - |
| `TMDB_API_KEY` | TMDB API Key | - |
| `TMDB_MIRROR_DOMAINS` | TMDB mirror domains | - |
| `BGMTV_USER_TOKEN` | BGM.tv user token | - |
| `BGMTV_MIRROR_DOMAINS` | BGM mirror domains | `api.bgm.tv,api.bangumi.tv,api.chii.in` |
| `DB_PATH` | Database file path | `ani-go.db` |
| `TV_BASE_PATH` | Anime root directory | `./TV/番剧` |
| `MOVIE_BASE_PATH` | Movie root directory | `./TV/剧场版` |
| `OVA_BASE_PATH` | OVA root directory | `./TV/OVA` |
| `PORT` | Web UI port | `20001` |

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
│   ├── parser/              # Natural language task parser
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
| `POST` | `/api/parse` | Parse natural language task |
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
| Phase 3: Web UI + RESTful API | ✅ Complete |
| Phase 4: AI + Multi-Downloader + Plugins + Multi-Source Sites | ✅ Complete |
| Phase 5: Multi-Platform Messaging | ✅ Complete (16 platforms) |
| Phase 6: Data Migration Tool | ✅ Complete |
| Phase 7: Frontend Polish + Search Fix | ✅ Complete |

## License

MIT License © xiaoyueRX
