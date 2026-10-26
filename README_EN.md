# Ani-Go

A self-hosted anime tracking and media organization tool.

Ani-Go follows anime releases through subscription feeds, sends matching resources to an existing download client, and organizes completed files into a media library. It is designed for home servers and NAS setups.

[简体中文](README.md) · [Releases](https://github.com/xiaoyueRX/Ani-Go/releases) · [Issues](https://github.com/xiaoyueRX/Ani-Go/issues) · [Deployment guide](docs/05_DEPLOYMENT.md)

![CI](https://github.com/xiaoyueRX/Ani-Go/actions/workflows/ci.yml/badge.svg)

## Features

- **Subscriptions and search:** Follow releases using Mikan RSS feeds; optional sources include Nyaa, ACG.RIP, and AnimeTosho for aggregated search.
- **Download management:** Connect to qBittorrent, Transmission, or Aria2 to submit tasks and check their status. Switch the active download client through settings.
- **Episode matching:** Parse release names using local rules and air dates. Optional AI-assisted parsing requires a separately configured provider.
- **File organization:** Generate folders and file names for anime series, films, and specials. Supports hard links, with a copy fallback when hard linking fails.
- **Web interface:** Manage subscriptions, downloads, schedules, and settings in a browser; Chinese and English UI translations are available.
- **Additional tools:** Notifications, built-in plugins, backups, and an MCP interface for selected management operations.

Ani-Go is not a BitTorrent client or a media player. Downloads require an external download client. Organized files can then be used by media servers such as Jellyfin or Emby.

## Quick start

### Docker Compose

You need Docker, Docker Compose, and at least one working download client.

```bash
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go
cp .env.example .env
```

Before starting:

1. Set your feed URL and download client connection details in `.env`.
2. Replace `/path/to/your/data:/data` in `docker-compose.yml` with an existing directory on your host.
3. Make sure the download client can access the same files, and the paths visible to Ani-Go match the mounted storage.

```bash
docker compose up -d
```

Visit `http://localhost:20001`. The initial credentials are `admin` / `admin`. **Change the password immediately.**

> The example Compose file maps port `20001` and stores application data under `/data`. Inside a container, `localhost` refers to that container—not your host or another container. Use an address reachable from Ani-Go when configuring a remote download client.

### Standalone binary

Download the archive for your platform from [Releases](https://github.com/xiaoyueRX/Ani-Go/releases), extract it, configure `.env` if needed, and start the application:

```bash
./anigo
```

Run `anigo.exe` on Windows. Ensure the process has permission to read and write its database, download, and media directories.

### Build from source

Requires **Go 1.25+** and **Node.js 24 with npm**. Build the frontend first; the Go binary embeds the built web assets.

```bash
cd web
npm ci
npm run build
cd ..

go build -o anigo .
./anigo
```

## Configuration

See [`.env.example`](.env.example) for a starting point.

| Variable | Purpose |
| --- | --- |
| `PORT` | Web/API port; defaults to `20001` |
| `MIKAN_RSS_URL` | Personal Mikan RSS feed |
| `DEFAULT_DOWNLOADER` | Active client: `qbittorrent`, `transmission`, or `aria2` |
| `QB_HOST` / `QB_USER` / `QB_PASS` | qBittorrent Web API endpoint and credentials |
| `TR_HOST` / `TR_USER` / `TR_PASS` | Transmission RPC endpoint and credentials |
| `ARIA2_HOST` / `ARIA2_SECRET` | Aria2 RPC endpoint and secret |
| `TV_BASE_PATH` | TV anime library directory |
| `MOVIE_BASE_PATH` / `OVA_BASE_PATH` | Movie and special-episode directories |
| `USE_HARDLINK` | Prefer hard links during organization |
| `DB_PATH` | SQLite database path |

Settings can come from environment variables, stored database settings, or built-in defaults. **Environment variables take precedence.** Values defined in a Compose `environment` block also override variables with the same names from `.env`. Review [docker-compose.yml](docker-compose.yml) before changing paths.

### Storage and hard links

A hard link can only be created when the source and destination are on the same filesystem. For Docker deployments, mount a common parent directory:

```yaml
volumes:
  - /volume1/data:/data
```

For example, use `/data/downloads` and `/data/media` and ensure both Ani-Go and your download client can access the required files. If hard linking fails because of mount boundaries or permissions, a copy may be created instead, using additional disk space.

See the [NAS deployment guide](docs/05_DEPLOYMENT.md) for further details.

## Security and limitations

- Change the default administrator password on first login.
- Do not expose port `20001` directly to the public internet. For remote access, prefer a VPN or an HTTPS reverse proxy with additional access controls.
- Keep passwords, RSS tokens, API keys, application databases, and backups out of public repositories.
- The MCP interface provides management operations; protect any remotely accessible endpoint with strong, separate credentials.
- You are responsible for configuring external download clients, feeds, and media services. Ani-Go cannot guarantee resource availability or download speed. Follow applicable laws and service terms.

## Development and documentation

The backend uses Go, GORM, and SQLite. The frontend uses Vue 3, TypeScript, Vite, and Tailwind CSS.

```bash
go test ./...
go test -race ./...
cd web && npm ci && npm run build
```

Related documentation:

- [Architecture](docs/01_ARCHITECTURE.md)
- [API reference](docs/API.md)
- [Plugin specification](docs/02_PLUGIN_SPEC.md)
- [MCP documentation](docs/03_MCP_SPEC.md)
- [Development plan](docs/DEVELOPMENT_PLAN_EN.md)

Please use [Issues](https://github.com/xiaoyueRX/Ani-Go/issues) for bugs and feature requests, or submit a pull request with improvements. Support for 115 cloud downloads is not currently built in; it is being discussed in [Issue #1](https://github.com/xiaoyueRX/Ani-Go/issues/1).

## License

Ani-Go is distributed under the [MIT License](LICENSE). Third-party dependencies and assets remain subject to their respective licenses.
