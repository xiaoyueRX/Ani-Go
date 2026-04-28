# AGENTS.md

This file provides guidance for AI assistants working on this repository.

## Tech Stack & Architecture
- **Language**: Go 1.22+
- **Database**: SQLite via GORM (`github.com/glebarez/sqlite` - pure Go driver, no CGO dependency)
- **Architecture**: Interface-driven (Hexagonal/Clean Architecture)
- **Core Interfaces**: Defined in `internal/core/interfaces.go` (Source, Downloader, MetadataProvider, Organizer, Notifier, EventBus)

## Common Commands
- **Run**: `go run .` (execute at project root)
- **Build**: `go build ./...`
- **Dependency Management**: `go mod tidy`

## Project-Specific Conventions & Notes
- **Database Driver**: Must use `github.com/glebarez/sqlite` instead of `gorm.io/driver/sqlite` to avoid CGO dependencies on Windows.
- **Config Management**: Loaded via `internal/config/config.go`. Environment variables take priority over defaults.
- **Sensitive Data**: Never hardcode tokens or passwords (e.g., `MIKAN_RSS_URL`, `QB_PASS`) in code. Always use environment variables.
- **File Encoding**: All files must be UTF-8 without BOM. Avoid using PowerShell's default `>` redirect as it may corrupt encoding.
- **Path Handling**: Pay attention to path mappings between Docker containers and the host (e.g., `/TV` inside the container maps to `/vol2/1000/TV` on the host).
- **Extensibility**: New features (e.g., new downloaders, new source sites) must implement the corresponding interfaces in `internal/core/interfaces.go` rather than directly modifying core logic.
- **Event Bus**: Use EventBus for inter-component communication (e.g., triggering file organization after download completion).
