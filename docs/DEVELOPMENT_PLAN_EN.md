# Ani-Go Development Plan

## 🎯 Core Positioning & Tech Stack
- **Language**: Go (single binary, fast startup, low memory footprint)
- **Deployment**: Docker one-click deployment, image size ~20MB; supports Windows, Mac, Linux, Docker, Go binary.
- **Open Source**: Published to GitHub under MIT License
- **Learning-Oriented**: Designed for programming beginners, learning by doing through AI assistance; code must be modular, well-commented, and conceptually explainable.

## 📥 Subscription & Resource Acquisition
- **Mikan Personal RSS Auto-Subscription**: Provide one personal RSS URL (with Token), the system automatically parses and syncs all Mikan web subscriptions.
- **Manual Search Subscription**: Web UI with built-in Mikan search; manually search for anime, choose subgroup, one-click subscribe.
- **Multiple Source Sites**: Mikan as primary, later expand to Nyaa, ACG.RIP, Anime Tosho, etc.
- **Subgroup Recognition & Filtering**: Identify different subgroup resources for the same anime, support priority settings or manual subgroup selection.

## ⬇️ Download & File Organization
- **Multiple Downloaders**: qBittorrent (required), Transmission, Aria2, expandable to 115 cloud, etc.
- **Historical Batch Completion (Core Pain Point)**: When detected downloaded episodes < total episodes, crawl Mikan anime detail pages for all historical torrents, filter by subgroup, deduplicate, and batch dispatch downloads.
- **Smart File Organization**: Auto-create complete directory structures (e.g., `Anime Title (Year)/Season 1/`, `Specials/`, standalone movie directories, etc.).
- **Fully Customizable Paths & Naming**: Template variable system (e.g., `{title_cn}`, `{title_en}`, `{year}`, `{season:02}`, `{ep:02}`, `{ext}`) to perfectly match existing fnOS/Jellyfin directory structures.
- **Media Library Compatible**: Organized files and directories must be directly scrapable by Jellyfin/fnOS.

## 🤖 AI & LLM Integration
- **Optional Toggle**: AI is an optional module; all core functions must work correctly when it is disabled.
- **Multi-Model Compatible**: Supports OpenAI-compatible API, Gemini, local Ollama, and other mainstream AI.
- **AI-Assisted Classification**: Auto-detect TV, Movie, OVA, Special, Extra, etc.
- **Smart Series Merging**: Cross-source-station, different seasons, movies/OVAs auto-identified as same series, unified into one parent directory with auto-season-sorting.
- **Natural Language Task Parsing**: Send messages (e.g., "subscribe Demon Slayer Season 2") via QQ/WeChat/Feishu/DingTalk/Telegram; AI parses into structured tracking tasks and auto-creates subscriptions.
- **Rule-Based Fallback**: When AI is disabled, uses TMDB Collection/Series ID, title normalization, BGM.tv associations, and regex rules.

## 🧩 Architecture Design & Extensibility
- **Interface-First**: All core modules abstracted as interfaces (Source / Downloader / Metadata / Organizer / Notifier); main program only depends on contracts, not implementations.
- **Highly Customizable Framework**: Configuration-driven behavior; pipeline steps are pluggable, order-adjustable, and individually replaceable.
- **Plugin System (V3 Focus)**: Fully open, highly customizable; supports third-party plugin repositories, online browsing and installation, complete plugin development documentation and SDK.
- **Event Bus**: Plugins/external scripts can listen to events (download start, completion, organization complete) and trigger custom logic.

## 🌐 Web UI & Interactive Experience
- **Anime Display**: Show anime cover, title, description, episode progress bar.
- **Core Pages**: Dashboard, subscription management, download queue, settings, plugin management.
- **Dual Metadata Sources**: Supports both TMDB and BGM.tv; users can switch preferred source in settings.
- **Manual Control**: Support pause/resume subscriptions, manually trigger historical completion for single anime, switch subgroups.
- **Timeout Warning & Suggestions**: If an anime hasn't finished downloading after 2+ days (possible dead torrent), system displays error hint on the anime card suggesting subgroup change.

## 🛠 Development, Deployment & Security Standards
- **Sensitive Data Isolation**: Mikan Token, downloader passwords strictly forbidden from hardcoding; must be injected via environment variables or `.env`.
- **Multi-Device Sync Development**: Based on Git workflow; switch computers by `git clone` + reading `docs/PROJECT_CONTEXT.md` to quickly restore full context.
- **CI/CD Automation**: Reserved GitHub Actions pipeline; auto-compile, build multi-architecture Docker images and push to GHCR on code push.
- **Encoding Standards**: All source files unified UTF-8 (no BOM); avoid Windows PowerShell default encoding corruption.
- **Data Migration**: Optionally add data migration from the original ani-rss (as optional enhancement).

---

## 📅 Development Roadmap

### Phase 0: Project Initialization & Architecture Setup
- [x] Determine project name (`Ani-Go`) and tech stack (Go + SQLite + GORM).
- [x] Initialize Go module and directory structure.
- [x] Define core interfaces (`internal/core/interfaces.go`).
- [x] Implement config loading system (environment variables first).
- [x] Implement database initialization and ORM models.
- [x] Create GitHub repository and complete first code push.
- [x] Create project memory document (`docs/PROJECT_CONTEXT.md`).
- [x] **Reference open source code**: Use AutoBangumi and original ani-rss as references for source code to avoid reinventing the wheel.
- [x] **Bilingual Documentation**: Generate pure Chinese and pure English versions for all `.md` and documentation files for international users.
- [x] **Chinese Comment Standard**: All Go source code comments use Chinese.

### Phase 1: Core Engine Implementation (MVP)
- [x] **Mikan RSS Parser**: Implement `Source` interface, parse Mikan personal RSS, auto-discover subscriptions.
- [x] **qBittorrent Client Integration**: Implement `Downloader` interface, interact with qBittorrent API.
- [x] **Basic Scheduler**: Implement periodic RSS polling and dispatch download tasks.
- [x] **Basic File Organization**: Implement simple rename and directory creation logic.
- [x] **EventBus**: Implement publish/subscribe pattern for decoupled inter-module communication.
- [x] **Title Parsing Enhancement**: 8 regex patterns covering Vol/【】/[]/SxxExx/.5/version/batch/special, referenced from ani-rss and AutoBangumi.

### Phase 2: Historical Completion & Metadata
- [x] **Mikan Full-Page Crawling**: Implement historical batch completion logic (core innovation).
- [x] **TMDB/BGM.tv Integration**: Implement `MetadataProvider` interface, fetch anime metadata.
- [x] **GFW Mirror/Proxy Support**: Multi-domain mirror auto-fallback for Mikan, BGM.tv, TMDB; GitHub proxy configuration.
- [x] **Supplement Scheduler** (ref AutoBangumi): Auto-detect episode gaps for incomplete subscriptions, crawl historical torrents for backfill.
- [x] **Dead Torrent Timeout Warning** (ref AutoBangumi): Auto-flag stalled downloads after N days, default 48h threshold adjustable via `stall_timeout_hours` setting.
- [x] **User-Customizable Regex** (ref AutoBangumi): Allow advanced users to add custom title parsing rules via settings table `custom_regex_N`, running alongside 8 built-in patterns with custom rules taking priority.

### Phase 3: Web UI & Deployment
- [x] **Basic Web UI**: Vue3 + Vite + DaisyUI login page, dashboard, subscription management, downloads queue, settings page.
- [x] **RESTful API**: Subscription CRUD (GET/POST/PUT/DELETE), download queue, settings management, supplement trigger.
- [x] **Docker Deployment**: Multi-stage Dockerfile (Node → Go → Alpine) + docker-compose.yml one-click startup.
- [x] **CI/CD**: Configure GitHub Actions for multi-arch (amd64/arm64) image building and GHCR push.

### Phase 4: Advanced Features (V2/V3)
- [x] **AI-Assisted Module**: Integrate LLMs for classification and series merging. Supports 4 protocols: OpenAI / Gemini / Claude / Ollama with auto-detection.
- [x] **Multiple Downloaders**: qBittorrent / Transmission / Aria2 implemented, switchable via env vars.
- [x] **Plugin System**: EventBus-driven Webhook + Shell script plugins with API management endpoints.
- [x] **Dead Torrent Detection**: Batch query for stalled episodes, frontend warning badges, configurable threshold.
- [x] **Custom Regex Patterns**: DB-stored user regex patterns with higher priority than built-in patterns, API reload.
- [x] **Multiple Source Sites**: Nyaa / ACG.RIP / AnimeTosho + MultiSource aggregator with dedup merge.
- [x] **Data Migration Tool**: Support importing data from original AutoBangumi.

### Phase 5: External Messaging Platform & AI Notification System
- [x] **Multi-Platform Messaging**: Unified `Notifier` interface with EventBus-driven auto-push, 16 platforms + `MultiNotifier` aggregated broadcast:
  - **Chinese IM**:
    - [x] **QQ**: OneBot protocol (NapCat/go-cqhttp/Lagrange/LLOneBot)
    - [ ] **WeChat Official Account**: Passive reply + customer service messages (pending)
    - [x] **WeCom (Enterprise WeChat)**: Webhook bot
    - [x] **Feishu/Lark**: Webhook bot
    - [x] **DingTalk**: Webhook bot
  - **International IM**:
    - [x] **Telegram**: Bot API + Markdown
    - [x] **Discord**: Webhook
    - [x] **Slack**: Webhook + Block Kit
    - [x] **LINE**: Messaging API / push message
    - [x] **WhatsApp**: Meta Cloud API (graph.facebook.com)
    - [ ] **Signal**: Bot API (pending)
  - **Push Notification Services**:
    - [x] **Email**: SMTP send notifications (goroutine + context timeout)
    - [x] **ServerChan**: HTTP API (WeChat push)
    - [x] **Bark**: HTTP API (iOS push)
    - [x] **Pushover**: HTTP API
    - [x] **Gotify**: HTTP API (self-hosted)
    - [x] **ntfy**: HTTP API (self-hosted open source push)
    - [x] **Matrix**: Client-Server API / PUT message
- [x] **Task Parser (Dual Mode)**:
  - [x] **Rule Engine (Default)**: Based on regex + keyword matching, zero dependencies, works without AI. Built-in patterns for subscribe/search/status commands.
  - [x] **AI Enhancement (Optional)**: Integrates OpenAI/Gemini/Ollama for higher accuracy (fuzzy expressions, synonym correction), auto-fallbacks to rule engine when AI is disabled.
- [x] **Notification Manager**: Listen to EventBus events (download complete/failed/supplement done, etc.), push notifications through all above platforms.
- [ ] **Notification Template System**: Support custom message templates, configurable per event type and platform. (pending)
- [ ] **Full Platform Integration Testing**: Verify connectivity and notification push for each platform. (partially pending)

### Phase 7: Frontend Polish + Search Fix + Schedule ✅
- [x] **Mikan Chinese Search Fix**: URL-encode Chinese keywords (`url.QueryEscape`) to fix Mikan 400 error; fallback CSS selector for Mikan's updated page structure.
- [x] **IconSax Component System**: Created `web/src/components/IconSax.vue`, 20+ Iconsax Linear icons replacing all inline SVGs and emoji.
- [x] **UI Polish**: Login page gradient background + brand icon, sidebar nav icons, subscription card SVG status indicators + hover effects, download list status badge icons, settings page restructure.
- [x] **Settings Page Restructure**: Vertical sidebar tabs, section grouping (downloader 4 groups / notification 5 groups), configured status ✓ badge, password visibility toggle, config progress indicator.
- [x] **New Season Schedule**: yuc.wiki data source, weekday grouping, standard poster images, 30-min auto-refresh.
- [x] **Search → Subscribe Flow**: Subtitle group selection modal + RSS URL auto-resolution + search cache.
- [x] **PWA Support**: manifest.json + service worker, Chrome/Edge installable as standalone app.
- [x] **Remember Password**: localStorage save, auto-fill on next visit.
- [x] **Episode Status Management**: `PUT /api/episodes/{id}/status` manual status toggle.
