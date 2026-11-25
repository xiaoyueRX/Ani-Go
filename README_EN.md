# Ani-Go

Follow anime releases on a NAS or home server. Ani-Go watches RSS feeds, sends new episodes to qBittorrent, Transmission or aria2, and organizes the finished files by series, season and episode for Jellyfin, Emby or another media server. The web interface is available in English and Simplified Chinese.

[简体中文](README.md) · [Deployment guide](docs/05_DEPLOYMENT_EN.md) · [Release downloads](https://github.com/xiaoyueRX/Ani-Go/releases/latest) · [Changelog](CHANGELOG.md) · [Report an issue](https://github.com/xiaoyueRX/Ani-Go/issues)

[![CI for main](https://github.com/xiaoyueRX/Ani-Go/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/xiaoyueRX/Ani-Go/actions/workflows/ci.yml?query=branch%3Amain)

## Quick start

### Docker Compose

You need Docker Compose, a working download client, and a shared directory both applications can read and write.

```bash
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go
cp .env.example .env
```

1. Create a data directory on your NAS and grant the container account read and write access. In `docker-compose.yml`, replace `source: /path/to/your/data` with its absolute path and keep `target: /data`. Compose checks that the host directory exists. The example uses UID/GID `1000:1000` by default.
2. Mount the storage under the same container path in your download client. For example, download source files into `/data/downloads` and point the TV library in your media server at `/data/media/番剧`. This keeps the originals available for seeding without indexing each episode twice.
3. Enter the download client address, credentials and RSS feed in Settings after signing in, or supply them in `.env` before startup. Between containers, use the service name or your NAS's LAN address. Inside a container, `localhost` refers to that container.

```bash
docker compose up -d ani-go
docker compose logs ani-go
```

Open `http://<NAS-LAN-address>:20001` and sign in as `admin`. On a fresh database, v0.8.0+ releases and the current source use `ANIGO_ADMIN_PASSWORD` for the initial password. If unset, they write a generated password to the first startup log. For historical releases such as `v0.7.0`, follow that version's login instructions in the startup log and change the initial password. Upgrades keep existing accounts.

The Compose file pulls the GHCR `latest` image, which follows commits to `main` that pass CI. For a pinned release, select its version tag. To run your checkout, change the service image to `ani-go:local`, enable `build: .`, and run `docker compose up -d --build ani-go`. See the changelog for release notes and changes across versions. [Compose file](docker-compose.yml) · [Changelog](CHANGELOG.md)

### Standalone binary

Starting with v0.8.0, official releases target x86-64 (`amd64`) and ARM64 on Windows and Linux, and Apple Silicon (`darwin-arm64`) on macOS. For a Linux NAS with a 64-bit Intel or AMD processor, choose `linux-amd64`.

Download the archive matching your operating system and CPU architecture. Use `checksums.txt` from the same release to verify it. In the extracted directory, configure `.env` and run `./anigo` (on Windows, `anigo.exe`). Use the same working directory on subsequent starts so the program can find its configuration and database at their relative paths. On Linux and macOS, the repository installer can also download and verify a release archive. [Installer](install.sh) · [Release downloads](https://github.com/xiaoyueRX/Ani-Go/releases/latest)

## Track your first series

1. In Settings, choose the active download client, enter its Web API address and credentials, and test the connection. Set a personal Mikan RSS feed, or supply a series-specific RSS feed on the subscription.
2. Search for a series and choose **Subscribe**. Selecting an episode release lets you review the series title, season, release group and resolution before tracking future releases. Mikan series entries and schedule dialogs offer a multi-select release-group picker with default-group matching.
3. Open Downloads to see the transfer. Open the subscription to check the episode's organization status after download. Leaving the subscription download directory empty uses the client's own save settings. Series, movies and specials can each have a separate library directory.
4. Open the subscription to inspect files marked **Needs review**. After correcting a mount, select the affected files and click **Recheck selected**. The page shows which files are queued and keeps failed items selected for retry. It skips files that need identification. For each of those, enter the season and episode or confirm the feature film. Use season 0 for specials.
5. Scan the organized directory with your media server. The Compose example enables hard linking to keep the source file available for seeding. Hard links require a shared filesystem. Organizing across filesystems copies the file and uses additional disk space.

Ani-Go can also search older releases for missing episodes, show broadcast schedules, apply naming templates, group series using TMDB metadata, and send notifications. AI-assisted parsing and MCP management are optional. [Architecture](docs/01_ARCHITECTURE_EN.md) · [Episode matching](docs/04_AUTOPARSER_SPEC.md) · [MCP setup](docs/03_MCP_SPEC_EN.md)

## Plugins

Plugins maintained by xiaoyueRX are marked **Official** in Ani-Go. Other authors' plugins are shown as community or third-party. Import an Ani-Go Webhook manifest to subscribe a service to events or give it its own credential field, actions and task list. Replace the example URL with the HTTPS endpoint operated by the plugin author. [Plugin author guide](docs/02_PLUGIN_SPEC_EN.md) · [Importable example](examples/plugins/cloud-tasks.json)

### 115 cloud downloads

NaJue requested a way to submit magnet links to 115 cloud downloads. Built-in support ships in v0.8.0+ as the official `cloud-115` plugin. [Original request #1](https://github.com/xiaoyueRX/Ani-Go/issues/1)

1. Open **Settings → Plugins & Automation** and enable **115 磁力云下载** (115 magnet cloud downloads). The plugin is disabled by default.
2. Save a user-authorized 115 Open Platform OAuth `access_token` with cloud download permission, or set `CLOUD115_ACCESS_TOKEN` in the deployment environment. The environment token takes precedence. Replace or remove it at its source.
3. On a search result with a magnet, choose **Send to plugin**, select **115 磁力云下载**, and confirm. You can also paste a magnet directly into the plugin card. Refresh the card to check progress. Files stay in your 115 account until you retrieve them. Local subscriptions use your configured download client.

When upgrading, an imported plugin that used the ID `cloud-115` receives a separate ID and keeps its author and settings. Check its source in the plugin list before enabling it. [115 authorization](https://www.yuque.com/115yun/open) · [Add a cloud task](https://www.yuque.com/115yun/open/zkyfq2499gdn3mty) · [List cloud tasks](https://www.yuque.com/115yun/open/av2mluz7uwigz74k)

## Configuration and maintenance

| Setting | Common variables |
| --- | --- |
| Web and storage | `PORT` defaults to `20001`. `DB_PATH` selects the SQLite database and `BACKUP_PATH` selects the backup directory. |
| RSS | `MIKAN_RSS_URL` sets your feed. `MIKAN_RSS_MODE=personal` matches existing subscriptions. `classic` can create subscriptions from the global feed. |
| Download client | `DEFAULT_DOWNLOADER` accepts `qbittorrent`, `transmission` or `aria2`. The respective connection settings are `QB_HOST/QB_USER/QB_PASS`, `TR_HOST/TR_USER/TR_PASS` and `ARIA2_HOST/ARIA2_SECRET`. |
| Library | `TV_BASE_PATH`, `MOVIE_BASE_PATH` and `OVA_BASE_PATH` set the directories for TV series, films and specials. `USE_HARDLINK` selects hard linking or moving. |
| Metadata and notifications | `TMDB_API_KEY` enables TMDB lookup and series grouping. The environment example also lists Telegram, WeChat, Feishu, Signal, Bark and other notification settings. |

Nyaa, ACG.RIP and AnimeTosho are disabled by default. Enable them in Settings or set `NYAA_ENABLED`, `ACGRIP_ENABLED` or `ANIMETOSHO_ENABLED`. Each source accepts a `*_DOMAIN` override. A domain set in the environment also enables that source unless an explicit switch overrides it.

Most startup settings prefer environment variables and fall back to saved values where supported. Runtime controls such as the organizer's hard-link switch also read settings saved in the web interface. In Compose, `environment` overrides the same keys in `env_file`. [Example environment](.env.example) · [NAS paths and permissions](docs/05_DEPLOYMENT_EN.md)

Before a container upgrade, confirm the database and backups are on persistent storage. The current Compose file stores backups at `/data/backups`. Before recreating an older container, inspect its `/app/data`. Copy any avatars from `/app/data/uploads` to `uploads` on the host share and retrieve any backups or historical logs you want to keep. For a deployment using the published image, run `docker compose pull ani-go` then `docker compose up -d ani-go`. Check `/api/health` for the version reported by the running service. [Upgrades and restore](docs/05_DEPLOYMENT_EN.md)

Change the initial administrator password in Settings. For remote access, use a VPN or an HTTPS reverse proxy with access controls. Give MCP its own credentials. Keep configuration files, databases and backups containing feed tokens, API keys or personal data private. Follow applicable terms when accessing media sources and services.

## Build from source

Install Go 1.25+ and Node.js 24 with npm. Run the following from the repository root. The Go build embeds the compiled Vue 3 frontend.

```bash
cd web
npm ci
npm test
npm run build
cd ..
go vet ./...
go test -race ./...
go build -o anigo .
./anigo
```

For frontend development, type checks, API contracts and planned work, see the project guides. [Frontend development](web/README_EN.md) · [API reference](docs/API.md) · [OpenAPI](docs/api.yaml) · [Development plan](docs/DEVELOPMENT_PLAN_EN.md)

## License

Ani-Go uses the MIT license. Third-party dependencies and assets retain their respective licenses. [LICENSE](LICENSE)
