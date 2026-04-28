# AGENTS.md

This file provides guidance for AI assistants working on this repository.

## Tech Stack & Architecture
- **Language**: Go 1.25+
- **Database**: SQLite via GORM (`github.com/glebarez/sqlite` - pure Go driver, no CGO dependency)
- **Key Dependencies**: `golang-jwt/jwt/v5` (JWT auth), `goquery` (HTML parsing), `golang.org/x/crypto` (Bcrypt)
- **Architecture**: Interface-driven (Hexagonal/Clean Architecture)
- **Core Interfaces**: Defined in `internal/core/interfaces.go` (Source, Downloader, MetadataProvider, Organizer, Notifier, EventBus)
- **Frontend**: Vue3 + Vite + TypeScript + TailwindCSS v4 + DaisyUI v5 (`web/` directory)

## Common Commands
- **Run**: `go run .` (execute at project root)
- **Build**: `go build ./...`
- **Test all**: `go test ./... -v`
- **Single test**: `go test -v -run TestName ./internal/package/`
- **Deps**: `go mod tidy`
- **Frontend dev**: `cd web && npm run dev`
- **Frontend build**: `cd web && npm run build`
- **GFW Go proxy**: `GOPROXY=https://goproxy.cn,direct go get ./...`

## Project Conventions

### Interface-First
New features (new downloaders, new source sites) must implement the corresponding interfaces in `internal/core/interfaces.go`. Do not modify core logic directly. The main program depends only on interfaces, not implementations.

### Database Driver
Must use `github.com/glebarez/sqlite` (pure Go driver). Forbidden: `gorm.io/driver/sqlite` (requires CGO, breaks cross-compilation).

### Config Management
Loaded via `internal/config/config.go`. Environment variables take priority over defaults. Runtime settings are stored in the database `settings` table and managed via `/api/settings`.

### Sensitive Data
Tokens, passwords must NEVER be hardcoded (e.g., `MIKAN_RSS_URL`, `QB_PASS`, `TMDB_API_KEY`, `BGMTV_USER_TOKEN`). Always inject via environment variables.

### File Encoding
All source files must be UTF-8 without BOM. Avoid PowerShell's default `>` redirect.

### Documentation Standard
- **Go source comments**: Chinese
- **Summary documents**: Bilingual (CN + EN copies, e.g. `README.md` + `README_EN.md`)
- **CLAUDE.md**: Chinese, used for Claude Code context

### GFW Network Environment
GitHub, Go proxy, Mikan and other overseas services may be blocked:
- Go modules: `GOPROXY=https://goproxy.cn,direct`
- Mikan: Built-in `proxyDomain` + `mirrorDomains` multi-domain auto-fallback
- BGM.tv: `api.bgm.tv` → `api.bangumi.tv` → `api.chii.in` tried in order
- TMDB: Configurable mirrors via `TMDB_MIRROR_DOMAINS`

### Path Handling
Pay attention to path mappings between Docker containers and the host (e.g., `/TV` inside container maps to `/vol2/1000/TV` on host).

### API Design
- Go 1.22+ `http.ServeMux` method-based routing (`GET /path`, `POST /path`)
- JWT Bearer Token auth (dynamically generated `crypto/rand` secret, regenerated each restart)
- All `/api/*` paths protected by AuthMiddleware (except `/api/login`)
- Request/response format: JSON (`Content-Type: application/json; charset=utf-8`)

### EventBus
Use EventBus for loosely-coupled inter-component communication (e.g., `download.completed` → trigger file organization → `file.organized`).

### GORM Gotchas
- `default:true` tag overrides zero-value `false` — use `db.Model().Update("field", false)` for boolean updates
- Soft delete via `gorm.Model`'s `DeletedAt` field
- Cascade deletes must be handled manually (e.g., delete episodes when deleting subscription)

## Current Project Status
- **Phase 0** ✅ — Project Initialization
- **Phase 1** ✅ — Core Engine MVP (31 tests)
- **Phase 2** ✅ — Historical Completion + Metadata + Mirror Support
- **Phase 3** 🚧 — Web UI + RESTful API (63 tests)
- **Phase 4** 📅 — AI + Multi-Downloader + Plugins
- **Phase 5** 📅 — Multi-Platform Messaging

## Key File Reference

| File | Purpose |
|------|---------|
| `internal/core/interfaces.go` | 6 core interfaces + 12 event constants + data types |
| `internal/config/config.go` | Config structs + env loading + defaults |
| `internal/database/models.go` | 5 ORM models |
| `internal/api/server.go` | HTTP route registration + middleware chain + server lifecycle |
| `internal/api/handlers.go` | 9 API handlers (subscription CRUD, downloads, settings) |
| `internal/source/mikan.go` | Mikan RSS parser + HTML detail page crawler + mirror fallback |
| `internal/scheduler/scheduler.go` | RSS polling + file organization + supplement scan + TriggerSupplement |
| `internal/downloader/qbittorrent.go` | qBittorrent Web API client |
| `internal/metadata/tmdb.go` | TMDB API v3 metadata provider |
| `internal/metadata/bangumi.go` | BGM.tv metadata provider |
| `main.go` | Startup flow: Config → JWT → DB → EventBus → Source → Downloader → Organizer → Scheduler → API |
