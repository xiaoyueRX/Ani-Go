# Ani-Go milestones and current work

[简体中文](DEVELOPMENT_PLAN.md) · [Release history](../CHANGELOG.md)

## Released milestones

| Stage | Delivered work |
| --- | --- |
| Early development through v0.5.x | RSS subscriptions, historical supplements, multiple downloaders and sources, organization, the web interface, migration and notifications. The v0.5.x releases added downloader switching, notification history and RSS inspection. |
| v0.6.0–v0.6.1 | Native MCP service and air-date-assisted episode matching, followed by notification templates, WeChat and Signal support, and TMDB collection grouping. |
| v0.6.2 | Scoped Go plugin contexts, reversible resource registration and lifecycle status. Core interfaces were defined for naming, filtering and parsing. The architecture guide identifies the current integration points. |
| v0.7.0 | qBittorrent 5.x pause/resume endpoints, SSE adjustments and the multi-platform release workflow. |

Published versions, archives and checksums are available on [GitHub Releases](https://github.com/xiaoyueRX/Ani-Go/releases).

## Current source tree

- **Downloads and organization.** RSS uses subscription feeds. Historical supplements apply title and subtitle-group matching. Download records, batch files and organization use episode information. A release-to-library check should compare the downloader's actual file with the final media path. [Architecture](01_ARCHITECTURE_EN.md)
- **Plugins and MCP.** The source implements official plugin attribution, service dependencies with suspension and recovery, declarative JSON panels and the official 115 magnet cloud plugin. Plugin controls report missing dependencies. Features available in a deployment depend on its build. [Plugin authoring](02_PLUGIN_SPEC_EN.md) · [MCP tools](03_MCP_SPEC_EN.md)
- **Release checks.** CI runs frontend tests and builds, Go race tests and compilation. Separate workflows produce container images and platform archives. [CI](../.github/workflows/ci.yml) · [Container builds](../.github/workflows/docker-build.yml) · [Release archives](../.github/workflows/release.yml)

Open feature requests and defects are tracked in [project issues](https://github.com/xiaoyueRX/Ani-Go/issues).
