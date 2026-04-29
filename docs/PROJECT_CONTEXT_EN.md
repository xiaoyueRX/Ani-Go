# Ani-Go Project Core Memory (Project Context)

> [!IMPORTANT]
> **This file is the "soul" of the project.** If you are starting work in a new environment or with a new AI assistant, please read this file first to get the complete project background and development constraints.

---

## 1. Developer & Environment Snapshot (Context)

### Background
- **Owner**: xiaoyueRX
- **Main Machine**: AMD Ryzen 5 9600X (PBO 200) / 32GB DDR5 6000 C28
- **Server**: PVE 9.1 (Systemd bound MAC) -> fnOS (Docker host)
- **Network Topology**:
  - Dormitory (NAT4) and home (IPv6+FRP) are physically isolated.
  - Sole gateway entry: Lucky v2.27.2 (HTTPS ports: 16601/50929).
  - **Critical Constraint**: Cross-subnet LAN broadcasts are strictly forbidden; access must go through Lucky reverse proxy or internal network tunneling.

### Storage Paths (fnOS)
- **TV/Anime Root**: `/vol2/1000/TV/Media/番剧`
- **Movie Root**: `/vol2/1000/TV/Media/剧场版`
- **Database Path**: `d:\pve\Ani-Go\ani-go.db` (Windows) / `/data/ani-go.db` (Docker)

---

## 2. Project Origin & Core Pain Points

### The Replacement Target: ani-rss
- **Pain Point 1**: RSS-only subscriptions; cannot handle older episodes not covered by RSS feeds.
- **Pain Point 2**: Poor database robustness, once suffered a massive logic crash due to a DNS failure.
- **Pain Point 3**: Subscription management is highly manual.

### Ani-Go's Ultimate Goals
1. **Mikan Personal RSS Full Sync**: One-click web subscription with automatic backend discovery and task creation.
2. **Historical Batch Completion (Soul Feature)**: Crawl Mikan anime detail pages to automatically fetch and download all historical torrents.
3. **Integrated File Organization**: Automatic renaming and directory hierarchy creation compliant with Jellyfin/fnOS scraping standards.
4. **Plugin & LLM Extensions**: Support external plugins and LLMs (OpenAI/Gemini/Ollama) for episode classification and series identification.

---

## 3. Technical Architecture (Stack)

- **Language**: Go 1.25+
- **Database**: SQLite (via GORM + pure Go driver `glebarez/sqlite`)
- **Key Dependencies**: `golang-jwt/jwt/v5` (JWT auth), `goquery` (HTML parsing), `golang.org/x/crypto` (Bcrypt)
- **Core Pattern**: **Interface-First**
  - `Source`: Resource fetching
  - `Downloader`: Download client interaction (qB/TR/Aria2)
  - `Metadata`: Anime metadata (TMDB/BGM.tv)
  - `Organizer`: File organization rules
- **Interaction**:
  - Vue3 + DaisyUI Web UI + RESTful API (JWT auth)
  - Plugin system (HTTP Sidecar pattern)

---

## 4. Current Development Progress

- [x] **Phase 0: Project Initialization** ✅
  - [x] Determine project name: `Ani-Go`
  - [x] Initialize Go module and directory structure
  - [x] Define core interfaces `internal/core/interfaces.go`
  - [x] Implement config loading system (environment variables first)
  - [x] Implement database initialization and ORM models (GORM)
  - [x] Create GitHub repository and complete first code push
- [x] **Phase 1: Core Engine MVP** ✅
  - [x] Mikan RSS parser (8 regex patterns)
  - [x] qBittorrent client integration
  - [x] Basic scheduler (periodic polling)
  - [x] Basic file organization
  - [x] EventBus
- [x] **Phase 2: Historical Completion & Metadata** ✅
  - [x] Mikan HTML full-page crawling (historical backfill)
  - [x] TMDB / BGM.tv metadata integration
  - [x] GFW mirror/proxy auto-fallback
  - [x] Supplement scheduler
- [x] **Phase 3: Web UI & Deployment** ✅
  - [x] Vue3 + DaisyUI frontend (login, subscription management, downloads, settings)
  - [x] RESTful API + JWT auth + go:embed frontend embedding
  - [x] Docker multi-stage build + CI/CD (GitHub Actions multi-arch images)
- [x] **Phase 4: AI + Multi-Downloader + Plugins** ✅
  - [x] AI multi-protocol (OpenAI/Google/Anthropic/Ollama)
  - [x] qBittorrent / Transmission / Aria2 multiple downloaders
  - [x] Plugin system (Webhook + Shell scripts)
  - [x] Multiple source sites (Nyaa/ACG.RIP/AnimeTosho + MultiSource aggregator)
- [x] **Phase 5: Multi-Platform Messaging** ✅
  - [x] 16-platform notification (Telegram/Discord/WeCom/Feishu/DingTalk/QQ/Slack/Matrix/LINE/WhatsApp/ServerChan/Bark/Pushover/Gotify/ntfy/Email)
  - [x] Natural language task parser (regex + AI fallback)
  - [x] EventBus auto-push + MultiNotifier aggregated broadcast
- [x] **Phase 6: Data Migration** ✅
  - [x] AutoBangumi SQLite data import tool
- **Tests**: 108 tests passing

---

## 5. TODOs & Notes

- **Token Protection**: `MIKAN_RSS_URL` must never appear in the codebase; must be injected via environment variables.
- **Encoding Standards**: All files must remain UTF-8 (no BOM) format. Do not use PowerShell's default `>` redirect.
- **Series Identification**: Future focus on tackling the "same series, different library names" merging logic (TMDB Collection approach).

---

*Last Updated: 2026-04-29*
