# Ani-Go

<div align="center">

<p align="center">
  <strong>Automated Anime Subscription, Download, Organization & Media Server Integration System</strong><br>
  <em>全自动番剧追番、下载、整理与媒体库刮削管理系统</em>
</p>

<p align="center">
  <a href="https://github.com/xiaoyueRX/Ani-Go/releases"><img src="https://img.shields.io/github/v/release/xiaoyueRX/Ani-Go?color=blue&style=flat-square" alt="Release"></a>
  <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/Vue-3.x-4FC08D?style=flat-square&logo=vue.js" alt="Vue 3">
  <img src="https://img.shields.io/badge/Docker-Ready-2496ED?style=flat-square&logo=docker" alt="Docker">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-green?style=flat-square" alt="License"></a>
</p>

<p align="center">
  <a href="#-features">Features</a> •
  <a href="#-quick-start">Quick Start</a> •
  <a href="#-configuration">Configuration</a> •
  <a href="#-plugin-ecosystem">Plugins</a> •
  <a href="#-media-library-structure">Media Library</a> •
  <a href="README.md">中文版本</a>
</p>

</div>

---

## 📖 Introduction

**Ani-Go** is a lightweight, high-performance, and modern all-in-one anime automation management system. Built with Go on the backend and Vue 3 on the frontend, it consumes only tens of megabytes of RAM and packages static assets directly inside a single standalone executable (`go:embed`).

Simply bind your personal **Mikan Project** RSS feed, and Ani-Go will continuously monitor seasonal anime updates, intelligently backfill missing historical episodes, and dispatch download tasks via **qBittorrent / Transmission / Aria2**. Once downloads finish, Ani-Go parses season and episode numbers and organizes files into standard library formats (Hardlink/Symlink/Move) ready for immediate scraping by **Jellyfin / Plex / Emby / fnOS**.

---

## ✨ Features

### 📺 Intelligent Seasonal & Daily Schedule
- **Multi-Source Architecture**:
  - **Mikan Project (Default)**: Official full seasonal anime data source, supporting cross-season historical navigation from 2000 to 2026 across all 4 quarters (Winter/Spring/Summer/Autumn).
  - **Bangumi (bgm.tv) Daily Broadcast**: Native integration with the official Bangumi calendar, **seamlessly synchronized with the Year and Season picker** to view historical seasonal schedules on demand.
  - **Yuc.wiki Plugin**: Built-in opt-in plugin (`yuc_schedule`). Once enabled in Settings, seamlessly switch to the Yuc.wiki schedule source.
- **Clean Aesthetic Layout**:
  - Displays pristine Monday-to-Sunday broadcasts with non-episodic noise removed.
  - Standardized poster aspect ratio (`aspect-[3/4.2]`), skeleton preloading, and resilient placeholder fallbacks to eliminate grid misalignment and layout holes.
  - Instant toggles for "Today Only", real-time text filter, and batch subscription mode.

### 🔄 Auto Tracking & Historical Backfill
- **Zero-Friction Tracking**: Sync directly from your Mikan RSS feed and automatically maintain active tracking.
- **Full Historical Backfill**: Subscribing to an anime mid-season automatically discovers and schedules all previously aired episodes, allowing you to catch up on an entire season with one click.
- **Subgroup & Filter Preferences**: Set specific subgroup preferences, language exclusions, and episode offsets per anime.

### ⬇️ Full-Featured Downloader Matrix
- **qBittorrent**: Deep native API integration, custom categories, and automated seeding lifecycle management (pause/remove finished torrents based on seeding ratio and time limits).
- **Transmission**: Lightweight torrent client support.
- **Aria2**: High-speed RPC direct download dispatching.
- **Real-Time Queue Dashboard**: Monitor progress, download/upload rates, seeding metrics, and error retries in real time.

### 🗂️ Standard Media Library Organization
- **Industry-Standard Naming Conventions**: Directory structure and file names adhere strictly to **Jellyfin / Plex / Emby / fnOS** guidelines:
  ```
  TV/番剧/
  └── Frieren Beyond Journey's End/
      └── Season 01/
          ├── Frieren Beyond Journey's End - S01E01.mp4
          └── Frieren Beyond Journey's End - S01E02.mp4
  ```
- **Multiple Organization Modes**:
  - **Hardlink (Recommended)**: Consumes zero extra disk space while allowing continuous seeding in your download client.
  - **Symlink**, **Copy**, or direct **Move**.
- **Smart Regex Parser**: Multi-tier pattern matching that strips fansub tags, release specifications, resolution indicators, and extracts bilingual titles and season/episode numbers.

### 🧩 Extensible Plugin Ecosystem
- **Built-in Plugins**:
  - 🔗 **Metadata Mapper (`metadata-mapping`)**: Maps Bangumi IDs directly to TMDB and IMDB entries.
  - 📁 **Standard Naming (`standard-naming`)**: Standardizes directory structures and filename formatting.
  - 📅 **Yuc.wiki Schedule (`yuc_schedule`)**: Provides opt-in Yuc.wiki seasonal data source in the schedule view.
- **Custom Webhooks**: Listen to core system events (subscription added, download completed, file organized) to easily integrate with third-party tools (Telegram, Discord, WeChat, scripts).

### 🌐 Smart Mirror Auto-Failover
- Built-in multi-mirror endpoint monitoring for Mikan, Bangumi, and TMDB.
- Background health checks measure latency and **automatically switch to the fastest responsive mirror** in milliseconds.

### 💻 Modern Responsive UI & PWA
- **Sticky Viewport Desktop Sidebar**: When scrolling through long seasonal schedules, the left navigation remains permanently locked in viewport view, never sliding away.
- **Mobile-First Bottom Dock**: Edge-to-edge frosted navigation bar with safe-area support and slide-over menu for mobile devices.
- **PWA Ready**: Installable as a native standalone app on desktop and mobile home screens.
- **Full Bilingual Support (i18n)**: Instant, zero-reload switching between Simplified Chinese and English across all UI views and settings.

---

## 🚀 Quick Start

### Method 1: Docker Compose (Recommended)

1. Clone the repository:
```bash
git clone --depth 1 https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go
```

2. Configure environment variables:
```bash
cp .env.example .env
# Edit .env with your MIKAN_RSS_URL, qBittorrent credentials, and media paths
vim .env
```

3. Launch containers:
```bash
docker compose up -d
```

4. Open your browser and navigate to `http://localhost:20001`:
   - **Default Username**: `admin`
   - **Default Password**: `admin`
   *(It is strongly recommended to change your password under Settings upon first login)*

---

### Method 2: Standalone Binary

Download the latest precompiled binary for your operating system from [Releases](https://github.com/xiaoyueRX/Ani-Go/releases):

```bash
# Launch Ani-Go (frontend is embedded inside the binary)
./anigo
```

Navigate to `http://localhost:20001` in your browser.

---

### Method 3: Build from Source

Prerequisites: Go 1.22+, Node.js 18+, npm

```bash
# 1. Clone the repository
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go

# 2. Build frontend assets
cd web
npm install
npm run build
cd ..

# 3. Build Go binary (embeds frontend dist)
go build -o anigo .

# 4. Run
./anigo
```

---

## ⚙️ Configuration

Refer to [`.env.example`](.env.example) for the complete list of options. Core variables:

| Variable | Description | Default | Example |
| :--- | :--- | :--- | :--- |
| `PORT` | Web UI & API listen port | `20001` | `20001` |
| `MIKAN_RSS_URL` | Mikan personal subscription RSS URL | None | `https://mikanani.me/RSS/MyBangumi?token=xxx` |
| `DEFAULT_DOWNLOADER`| Default download client | `qbittorrent`| `qbittorrent` / `transmission` / `aria2` |
| `QB_HOST` | qBittorrent WebUI host | `http://localhost:8081` | `http://192.168.1.10:8081` |
| `QB_USER` | qBittorrent WebUI username | `admin` | `admin` |
| `QB_PASS` | qBittorrent WebUI password | `adminadmin` | `your_password` |
| `TV_BASE_PATH` | Base directory for organized media | `./TV/番剧` | `/media/anime` |
| `ORGANIZE_MODE` | File organization mode | `hardlink` | `hardlink` / `symlink` / `move` / `copy` |
| `POLL_INTERVAL` | RSS polling interval | `30m` | `15m`, `30m`, `1h` |
| `CLEANUP_ENABLED` | Enable automatic seed cleanup | `true` | `true` / `false` |
| `CLEANUP_MIN_SEED_TIME` | Minimum seed retention duration | `48h` | Clean up after seeding this long |
| `CLEANUP_MIN_RATIO` | Minimum share ratio threshold | `1.0` | Clean up once ratio reaches 1.0 |

---

## 📡 API & Developer Resources

Ani-Go provides comprehensive RESTful APIs for custom automation and client integrations:
- **API Documentation**: See [docs/API.md](./docs/API.md)
- **OpenAPI Specification**: See [docs/api.yaml](./docs/api.yaml)
- **Interactive Documentation**: Accessible at `http://localhost:20001/api/login` when running.

---

## 🤝 Credits

- [Mikan Project](https://mikanani.me/) — Exceptional anime RSS feeds and release catalog.
- [Bangumi](https://bgm.tv/) — Comprehensive ACG metadata and community rankings.
- [Yuc.wiki](https://yuc.wiki/) — Dedicated seasonal anime timetable curation.
- [Vue.js](https://vuejs.org/) & [DaisyUI](https://daisyui.com/) & [TailwindCSS](https://tailwindcss.com/)

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).
Contributions, issues, and feature requests are welcome!
