# Ani-Go 目录索引 · Directory map

| 路径 / Path | 职责 / Responsibility |
| --- | --- |
| [main.go](../main.go)、[internal/app](../internal/app) | 启动、关闭与服务装配 · Startup, shutdown and service wiring |
| [embed.go](../embed.go)、[web](../web) | Vue 前端、前端回归与嵌入资源 · Vue UI, frontend tests and embedded assets |
| [internal/config](../internal/config)、[internal/database](../internal/database) | 配置解析、SQLite 模型和迁移 · Configuration, SQLite models and migrations |
| [internal/source](../internal/source)、[internal/search](../internal/search)、[internal/metadata](../internal/metadata) | 资源站、搜索聚合、元数据 · Release sources, search and metadata |
| [internal/parser](../internal/parser)、[internal/ai](../internal/ai) | 标题、任务及可选 AI 解析 · Title and task parsing, optional AI |
| [internal/scheduler](../internal/scheduler)、[internal/downloader](../internal/downloader)、[internal/organizer](../internal/organizer) | 订阅调度、下载器、媒体整理 · Scheduling, downloaders and organization |
| [internal/core](../internal/core)、[internal/event](../internal/event)、[internal/plugin](../internal/plugin) | 接口与服务、事件分发、插件运行时 · Interfaces and services, event delivery, plugin runtime |
| [internal/cloud115](../internal/cloud115)、[examples/plugins](../examples/plugins) | 官方 115 客户端与可导入社区示例 · Official 115 client and importable community example |
| [internal/notifier](../internal/notifier)、[internal/backup](../internal/backup)、[internal/migrate](../internal/migrate)、[internal/update](../internal/update) | 通知、备份、导入和更新 · Notifications, backups, imports and updates |
| [internal/api](../internal/api)、[internal/auth](../internal/auth)、[internal/mcp](../internal/mcp)、[internal/httpx](../internal/httpx) | HTTP 管理、鉴权、MCP、HTTP 辅助函数 · HTTP management, authentication, MCP and HTTP helpers |
| [cmd](../cmd)、[.github/workflows](../.github/workflows) | 独立命令行工具与构建工作流 · Standalone utilities and build workflows |

运行说明见 [README](../README.md)；架构定位见 [系统架构](01_ARCHITECTURE.md)。For setup and design, see the [English README](../README_EN.md) and [architecture](01_ARCHITECTURE_EN.md).
