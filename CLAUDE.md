# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Common Commands

```bash
# Build and run (Go backend)
go run .                        # Run the server
go build ./...                  # Compile all packages
go test ./... -v                # Run all tests
go test -v -run TestHash ./internal/auth/  # Run single test

# Frontend (web/)
cd web && npm install           # Install dependencies (first time)
cd web && npm run dev           # Dev server (Vite hot reload)
cd web && npm run build         # Production build

# Dependencies
go mod tidy                     # Sync Go dependencies
```

## Architecture

**Interface-first design.** All core modules are abstracted behind interfaces defined in `internal/core/interfaces.go`: `Source`, `Downloader`, `MetadataProvider`, `Organizer`, `Notifier`, `EventBus`. Never depend on a concrete implementation — always depend on the interface. Adding a new downloader (e.g. Transmission) or source site means implementing the interface, not modifying existing logic.

**Startup flow** (`main.go`): Config load → JWT secret init (crypto/rand 32B, ephemeral) → DB init + default admin user (Bcrypt) → EventBus → Mikan source → qBittorrent downloader → Organizer → Scheduler (RSS polling loop) → HTTP server (graceful shutdown).

**API routing**: Go 1.22+ `http.ServeMux` method-based patterns (`GET /api/subscriptions`, `POST /api/subscriptions`, `GET /api/subscriptions/{id}`). All `/api/*` paths require JWT auth except `/api/login`. The `Server` struct in `internal/api/server.go` holds dependencies (downloader, supplement trigger callback).

## Key Conventions

- **Windows dev / Linux deploy**: The developer works on Windows (PowerShell) but deploys to a PVE LXC container (Debian). Code must compile and run on both. The SQLite driver `github.com/glebarez/sqlite` is a pure Go driver — `gorm.io/driver/sqlite` requires CGO and breaks cross-compilation.
- **GFW 网络环境**: GitHub、Go 代理、Mikan 等境外服务可能被墙。Go 模块下载用 `GOPROXY=https://goproxy.cn,direct`。Mikan、BGM.tv、TMDB API 均已内置多域名镜像自动回退机制（`tryMirrors` 方法）。
- **Chinese comments**: All Go source comments are in Chinese. Summary documentation files are bilingual (CN + EN copies, e.g. `README.md` + `README_EN.md`).
- **GitHub push**: GFW blocks GitHub; a VPN (TUN mode global proxy) is needed for `git push`, `gh`, and cloning from GitHub.
- **JWT auth**: Secret is dynamically generated each startup via `crypto/rand` (never hardcoded). RBAC by default is token-only (users table stores Bcrypt hashes). The `/api/me` endpoint uses `extractToken()` to re-validate even though AuthMiddleware already checked it — intentional belt-and-suspenders.
- **Middleware chain**: `ProxyHeadersMiddleware` (Lucky v2.27.2 reverse proxy compatibility via X-Forwarded-*) → `CORSMiddleware` → `AuthMiddleware` (bypasses `/api/login`, protects all other `/api/*`).

## Web Frontend (`web/`)

Vue3 + Vite + TypeScript + TailwindCSS v4 + DaisyUI v5. The frontend is NOT embedded in the Go binary yet — it's served separately during development. Router guard (`beforeEach`) checks localStorage for a JWT token; Axios interceptor injects `Authorization: Bearer <token>` and intercepts 401 to redirect to `/login`. The `index.html` should have `data-theme="dark"` for DaisyUI dark mode.

**Node 24 ESM compatibility**: `@plugin "daisyui"` in CSS fails on Node 24. Use `@import "daisyui/daisyui.css"` instead.

## Title Parsing (Mikan RSS)

The Mikan RSS title parser in `internal/source/mikan.go` is the most regex-heavy module. It uses 8 patterns for episode detection: `SxxExx`, dash-ep, Vol, Chinese episode (第X話), EPxx, #xx, 【xx】, [xxv2]. Special cases: `.5` episodes, batch detection, Chinese numeral conversion (一→1, 二十→20). `ParseMikanTitle()` extracts: subgroup, title, season, episode, resolution, version, and flags (batch/special).

## Mikan HTML Crawling

`parseMikanDetailHTML()` in `internal/source/mikan.go` uses goquery (Go equivalent of Jsoup) to parse Mikan anime detail pages. CSS selectors reference ani-rss:
- Subtitle groups: `.leftbar-item a.subgroup-name` (with `data-anchor` attribute)
- Torrent tables: `a[name="{id}"]` → `NextAllFiltered("table")` → `tbody tr`
- Magnet links: `a[data-clipboard-text]` (40-char hex hash extracted via regex)
- File sizes parsed by `parseSize()` ("1.2 GB", "500 MB", "100 KB" → bytes)
- `tryMirrors()`: attempts proxyDomain → primary domain → mirrorDomains in order, returns first 200

## API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/login` | Login, returns JWT token |
| `GET` | `/api/me` | Current user info |
| `GET` | `/api/health` | Health check (no auth) |
| `GET` | `/api/subscriptions` | List subscriptions (?enabled=&completed=) |
| `POST` | `/api/subscriptions` | Create subscription |
| `GET` | `/api/subscriptions/{id}` | Subscription detail + episodes |
| `PUT` | `/api/subscriptions/{id}` | Partial update (pointer fields) |
| `DELETE` | `/api/subscriptions/{id}` | Delete + cascade episodes |
| `POST` | `/api/subscriptions/{id}/trigger-supplement` | Manual backfill trigger |
| `GET` | `/api/downloads` | qBittorrent download queue |
| `GET` | `/api/settings` | All settings (key-value) |
| `PUT` | `/api/settings` | Batch update settings |

## GORM Gotchas

- `default:true` tag causes zero-value `false` to be overwritten on Create. For seeding `Enabled: false` in tests, use `db.Model().Update("enabled", false)` after Create.
- Soft delete via `gorm.Model.DeletedAt` — records are hidden, not removed.
- Cascade deletes not automatic — delete related records manually (e.g. episodes when deleting subscription).

## Current Phase

Phase 1 (Core Engine MVP) is complete. Phase 2 (history backfill + metadata + GFW mirrors) is complete. Phase 3 (Web UI + RESTful API) is in progress — backend CRUD API done, frontend login + dashboard done, subscription management UI remaining. See `docs/DEVELOPMENT_PLAN.md` for the full roadmap. **Tests: 63 passing across all packages.**
