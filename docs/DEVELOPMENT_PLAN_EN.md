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
- [ ] **Mikan RSS Parser**: Implement `Source` interface, parse Mikan personal RSS, auto-discover subscriptions.
- [ ] **qBittorrent Client Integration**: Implement `Downloader` interface, interact with qBittorrent API.
- [ ] **Basic Scheduler**: Implement periodic RSS polling and dispatch download tasks.
- [ ] **Basic File Organization**: Implement simple rename and directory creation logic.

### Phase 2: Historical Completion & Metadata
- [ ] **Mikan Full-Page Crawling**: Implement historical batch completion logic (core innovation).
- [ ] **TMDB/BGM.tv Integration**: Implement `MetadataProvider` interface, fetch anime metadata.
- [ ] **Enhanced File Organization**: Support custom path templates, improve series merging rules.

### Phase 3: Web UI & Deployment
- [ ] **Basic Web UI**: Implement dashboard, subscription list, settings page.
- [ ] **RESTful API**: Provide backend APIs for Web UI.
- [ ] **Docker Deployment**: Write Dockerfile and docker-compose.yml.
- [ ] **CI/CD**: Configure GitHub Actions for automatic image building.

### Phase 4: Advanced Features (V2/V3)
- [ ] **AI-Assisted Module**: Integrate LLMs for classification and series merging.
- [ ] **Multiple Downloaders/Source Sites**: Support Transmission, Aria2, Nyaa, etc.
- [ ] **Plugin System**: Design and implement open plugin architecture and event bus.
- [ ] **Data Migration Tool**: Support importing data from original ani-rss.

### Phase 5: External Messaging Platform & AI Notification System
- [ ] **EventBus Implementation**: Implement event bus (`internal/event/bus.go`) with publish/subscribe support.
- [ ] **Multi-Platform Messaging**: Unified `Messenger` interface supporting both Chinese and international mainstream platforms:
  - **Chinese IM**:
    - [ ] **QQ**: Reverse WebSocket (go-cqhttp / Lagrange compatible)
    - [ ] **WeChat Official Account**: Passive reply + customer service messages
    - [ ] **WeCom (Enterprise WeChat)**: Webhook bot + app messages
    - [ ] **Feishu/Lark**: Webhook + event subscription + official SDK
    - [ ] **DingTalk**: Webhook bot + message push
  - **International IM**:
    - [ ] **Telegram**: Bot API (long polling getUpdates)
    - [ ] **Discord**: Bot + Webhook (discordgo)
    - [ ] **Slack**: Socket Mode + Web API (slack-go)
    - [ ] **LINE**: Messaging API
    - [ ] **WhatsApp**: Cloud API (Meta)
    - [ ] **Signal**: Bot API
  - **Push Notification Services**:
    - [ ] **Email**: IMAP receive commands + SMTP send notifications
    - [ ] **ServerChan**: HTTP API (sctapi.ftqq.com, WeChat push)
    - [ ] **Bark**: HTTP API (iOS push)
    - [ ] **Pushover**: HTTP API
    - [ ] **Gotify**: WebSocket + HTTP API (self-hosted)
    - [ ] **ntfy**: HTTP API (self-hosted open source push)
- [ ] **Task Parser (Dual Mode)**:
  - [ ] **Rule Engine (Default)**: Based on regex + keyword matching, zero dependencies, works without AI. Built-in patterns for subscribe/search/status commands.
  - [ ] **AI Enhancement (Optional)**: Integrates OpenAI/Gemini/Ollama for higher accuracy (fuzzy expressions, synonym correction), auto-fallbacks to rule engine when AI is disabled.
- [ ] **Notification Manager**: Listen to EventBus events (download complete/failed/supplement done, etc.), push notifications through all above platforms.
- [ ] **Notification Template System**: Support custom message templates, configurable per event type and platform.
- [ ] **Full Platform Integration Testing**: Verify connectivity, command parsing, and notification push for each platform; all 17 platforms must pass.
