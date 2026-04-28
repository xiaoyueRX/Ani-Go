# Ani-Go Project Transfer Context

> This file helps quickly restore working context after migrating from a Windows dev machine to the home server PVE LXC cloud VS Code environment.

## Quick Recovery (3 Steps)

```bash
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go
cp .env.example .env
# Edit .env and fill in actual configuration
```

## Current Status

- **Branch**: main
- **Phase 0**: Complete
- **Phase 1 (Core Engine MVP)**: Complete — 31 tests passing, Mikan RSS parser, qBittorrent client, scheduler, organizer, EventBus, 8 regex title parsing patterns
- **Phase 2 (Historical Completion + Metadata)**: Complete — Mikan HTML parser (goquery), TMDB/BGM.tv providers, mirror/proxy architecture for GFW, supplement scheduler
- **Phase 3 (Web UI + RESTful API)**: In progress — Vue3 frontend (Login + Dashboard), RESTful API (subscription CRUD, downloads, settings)
- **Tests**: 63 passing across all packages

## Tech Stack

- Go 1.25+
- SQLite (pure Go driver `github.com/glebarez/sqlite`, no CGO)
- GORM ORM
- goquery (HTML parsing, Go equivalent of Jsoup)
- Architecture: Interface-driven (Source / Downloader / MetadataProvider / Organizer / Notifier / EventBus)
- Frontend: Vue3 + Vite + TypeScript + TailwindCSS v4 + DaisyUI v5
- JWT auth with `golang-jwt/jwt/v5`, Bcrypt password hashing

## Key Files

| File | Description |
|------|-------------|
| `main.go` | Entry point, prints banner, loads config, initializes all modules |
| `internal/core/interfaces.go` | 6 core interfaces + 12 event constants |
| `internal/config/config.go` | Config loading (env vars take priority) |
| `internal/database/db.go` | GORM initialization + AutoMigrate |
| `internal/database/models.go` | 5 ORM models (Subscription, Episode, DownloadRecord, Setting, User) |
| `internal/source/mikan.go` | Mikan RSS parser + HTML detail page crawler |
| `internal/scheduler/scheduler.go` | Scheduled tasks: RSS polling, file organization, supplement scanning |
| `internal/api/server.go` | HTTP API server with JWT auth middleware |
| `internal/api/handlers.go` | RESTful API handlers: subscription CRUD, downloads, settings |
| `internal/metadata/tmdb.go` | TMDB API v3 metadata provider |
| `internal/metadata/bangumi.go` | BGM.tv metadata provider |
| `internal/downloader/qbittorrent.go` | qBittorrent Web API client |
| `AGENTS.md` / `AGENTS_EN.md` | AI assistant guidelines (CN/EN) |
| `CLAUDE.md` | Claude Code guidance |
| `docs/DEVELOPMENT_PLAN.md` | Full 5-phase development roadmap (CN/EN) |
| `docs/PROJECT_CONTEXT.md` | Project core memory (CN/EN) |

## Key Constraints

- Tokens/passwords strictly must NOT be hardcoded; must use env vars
- File encoding: UTF-8 without BOM
- Database driver: must use `github.com/glebarez/sqlite`
- Go source comments in Chinese, documentation bilingual (CN + EN)
- New features must implement corresponding interfaces
- GFW environment: use `GOPROXY=https://goproxy.cn,direct`, mirror domains configured for Mikan/BGM/TMDB

## Server Environment

- PVE 9.1 → fnOS Docker host
- Storage: `/vol2/1000/TV/Media/番剧` (TV) / `/vol2/1000/TV/Media/剧场版` (Movie)
- Network: Lucky v2.27.2 reverse proxy, HTTPS ports 16601/50929
- Cross-subnet LAN broadcast prohibited; must go through Lucky reverse proxy or internal tunnel
