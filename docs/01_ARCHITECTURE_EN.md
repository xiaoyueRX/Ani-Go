# Ani-Go architecture

[简体中文](01_ARCHITECTURE.md) · [Directory map](DIRECTORY_STRUCTURE.md)

## From a subscription to a media library

1. The scheduler reads enabled subscriptions and RSS configuration. It matches releases by feed, title and filters such as subtitle group. A historical supplement can also search other configured sources by title. [Subscription matching](../internal/scheduler/subscription_feeds.go) · [Scheduler](../internal/scheduler/scheduler.go)
2. Local rules recognize seasons, episodes, specials and batches. Ambiguous results may use air-date matching or the optional AI service when their prerequisites are met. Mikan, Bangumi and TMDB each have their own identifiers. [Episode matching](04_AUTOPARSER_SPEC.md)
3. Ani-Go records download tasks and sends them to the selected qBittorrent, Transmission or Aria2 client. The scheduler reads the completed file paths from that client. A single batch can supply several episodes. [Download queue](../internal/scheduler/queue.go) · [Downloader adapters](../internal/downloader)
4. The organizer applies the naming template and registered naming hooks, and constructs paths for TV episodes, films and specials. Hard-link mode retains the source and falls back to copying when necessary. Move mode relocates the source. A media server scans the resulting library. [Organizer](../internal/organizer/organizer.go) · [Preview](../internal/organizer/preview.go)

Single, batch and MCP subscription requests share the same database transaction. Different seasons of one title can be tracked separately. Concurrent requests for the same title and season create one record. With multiple active seasons of the same title, a shared feed or historical search needs an explicit season or a verified series source. A separate feed attached to a single season can carry releases without season tags. Schedule entries link by provider ID or by a title that identifies a single subscription. [Subscription creation](../internal/database/subscription.go) · [Schedule matching](../internal/api/schedule_subscriptions.go)

Saving a polling interval reschedules its timer immediately. Manual and scheduled requests for the same subscription share an ongoing history scan. Progress counts distinct regular episodes in the subscribed season, including organized episodes and episodes marked complete manually. Shutdown cancels and waits for admitted jobs before releasing plugins. [Scheduler lifecycle](../internal/scheduler/runtime.go) · [Episode progress](../internal/database/progress.go)

## Runtime and control interfaces

The host wires the sources, metadata provider, downloader, SQLite database, scheduler and plugin manager. Plugins can listen for organization events or declare required services. The enabled switch records user intent. Runtime status shows whether the plugin is available. A missing service suspends a consumer until its requirements are present again. [Startup](../main.go) · [Plugin specification](02_PLUGIN_SPEC_EN.md)

The Go binary embeds the web interface. The UI uses REST. MCP clients connect over SSE and invoke registered tools. The event bus dispatches asynchronously within the process, while naming hooks run synchronously during organization. Backup and restore use a separate host service. [HTTP routes](../internal/api/server.go) · [MCP](03_MCP_SPEC_EN.md) · [Backup](../internal/backup)
