# Ani-Go repository development guide

[简体中文](AGENTS.md)

## Architecture and builds

Ani-Go fetches RSS feeds, parses episodes, schedules downloads and organizes completed files for media libraries such as Jellyfin. With hard links enabled, a failed link can fall back to copying and preserve the source. Disabling hard links moves the file. Prefer plugins for extensions. Ani-Go owns its Go service registry and plugin lifecycle. Community authors declare webhooks and panels in Ani-Go JSON manifests. The server assigns official provenance from its maintained registry. [Architecture](docs/01_ARCHITECTURE_EN.md) · [Plugin protocol](docs/02_PLUGIN_SPEC_EN.md)

Use Go 1.25+, the standard HTTP router, GORM and the pure-Go SQLite driver `github.com/glebarez/sqlite`. Production builds must support `CGO_ENABLED=0`. The frontend uses Node 24, Vue 3, TypeScript, Vite, Tailwind CSS, DaisyUI and Lucide. Build the frontend first so Go embeds the matching UI.

```sh
(cd web && npm ci && npm test && npm run build)
go test -race ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o ani-go .
```

## Development conventions

- Use Simplified Chinese for communication, Go comments and commit messages. Write UTF-8 without a BOM. Maintain the existing bilingual guides together. Update HTTP documentation, examples and OpenAPI when contracts change.
- Access core services through the interfaces in `internal/core`. Built-in plugins register subscriptions, services and background tasks with their scope. Verify cleanup on disable. Use the author guide for webhook, action and credential contracts.
- Initialize configuration from defaults and the deployment environment, then merge saved runtime settings. Field-specific precedence and runtime updates are defined in `internal/config/config.go` and dedicated handlers. Use deployment variables or the dedicated settings UI for credentials. Keep credentials out of source, ordinary logs and manifest exports.
- The first startup creates an administrator. The JWT signing key persists and passwords use bcrypt. The API guide describes public routes, MCP credentials and SSE authentication.
- Explicitly write false when updating GORM booleans with defaults. Check related records, cleanup and restore behavior when handling soft deletion.
- Run backend checks on Linux or WSL and verify the target OS for platform-specific changes. Choose module proxies and mirrors appropriate to the deployment. Use Lucide icons. The frontend guide documents the dev proxy, tests and builds.
- The NAS idle memory target is 15–30 MB, with 0.0% idle CPU. Report measurements with hardware and conditions. Align the changelog, version source and build artifacts for releases.
- CI builds the frontend and runs Go race tests for main-branch changes and pull requests. The image and release workflows define their own publication triggers.

## References

[Installation](README_EN.md) · [Design goals](docs/00_DESIGN_PHILOSOPHY.md) · [Directories](docs/DIRECTORY_STRUCTURE.md) · [HTTP API](docs/API.md) · [MCP](docs/03_MCP_SPEC_EN.md) · [Frontend](web/README_EN.md) · [Development plan](docs/DEVELOPMENT_PLAN_EN.md) · [Release history](CHANGELOG.md)

## Checking the impact of a change

Run the following command from the repository root to create or refresh the local GitNexus index. The generated graph stays under `.gitnexus/` on the development machine.

```sh
npx gitnexus analyze --skip-agents-md --skip-skills
```

Before changing a function, type or method, use GitNexus `impact` to identify upstream callers and affected flows. For a HIGH or CRITICAL result, explain which entry points are affected and cover them in regression checks. Check Vue template bindings, event registrations and other dynamic callers in the source as well. Use GitNexus symbol rename for cross-module refactors. Before committing, check the staged changes:

```sh
npx gitnexus detect-changes --scope staged
git diff --cached --check
```

[Caller and impact analysis](.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md) · [Graph commands](.claude/skills/gitnexus/gitnexus-cli/SKILL.md)
