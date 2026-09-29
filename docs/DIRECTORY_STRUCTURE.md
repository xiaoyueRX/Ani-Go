# Ani-Go 目录结构说明

本规范供开发人员与架构维护查阅，明确工程各层级模块的边界与职责划分。

## 🌟 核心根目录
- `CODEX.md`：OpenAI Codex / CodeX CLI 接管全局上下文与工作流转移圣经。
- `AGENTS.md` / `AGENTS_EN.md`：AI 智能体开发规范与系统指令。
- `CLAUDE.md`：Claude Code 上下文与开发指引。
- `.cursorrules`：代码编辑器规则（纯 Go 红线、架构边界约束等）。
- `README.md` / `README_EN.md`：中英双语项目主页与快速开始指南。
- `CHANGELOG.md`：标准化版本发布与变更履历。
- `main.go`：应用程序生命周期入口与总装逻辑。
- `embed.go`：静态资源 `//go:embed` 内嵌粘合模块。
- `go.mod` / `go.sum`：Go 依赖管理（Go 1.25+）。
- `Dockerfile` / `docker-compose.yml`：多架构容器化构建与生产运行编排模板。
- `install.sh`：Linux / macOS 一键自动化安装脚本。

## ⚙️ 后端目录 (`internal/`)
后端采用微内核与高内聚包设计：
- `internal/core/`：核心抽象接口（`Source`, `Downloader`, `Organizer`, `EventBus` 等）。
- `internal/api/`：RESTful API 路由定义、Handlers 与 WebSocket 控制器。
- `internal/config/`：配置加载器（三级优先决议律：环境变量 > 数据库设置 > 内置默认值）。
- `internal/database/`：SQLite GORM 实体定义与数据库迁移（纯 Go 驱动）。
- `internal/downloader/`：下载器适配层（qBittorrent / Transmission / Aria2 / DynamicDownloader）。
- `internal/mcp/`：原生 Model Context Protocol (MCP) Server（SSE + Bearer 鉴权）。
- `internal/organizer/`：文件规范化整理器（硬链接/移动、路径防越界安全加固）。
- `internal/parser/`：种子标题消歧与解析器（本地正则、Bangumi 放送时间戳对齐算法）。
- `internal/plugin/`：微内核插件管理器与内置插件集（物理断电隔离）。
- `internal/source/`：资源站采集驱动（Mikan, Nyaa, ACG.RIP, AnimeTosho）。
- `internal/metadata/`：番剧元数据客户端（Bangumi, TMDB）。
- `internal/notifier/`：16 渠道消息聚合推送引擎与投递监控。
- `internal/ai/`：外部大模型协议客户端（DeepSeek / OpenAI / Gemini / Ollama）。
- `internal/event/`：内存高性能发布-订阅事件总线。
- `internal/httpx/`：统一出站 HTTP 客户端（连接池复用、超时保护）。
- `internal/scheduler/`：定时任务轮询与计划任务编排。
- `internal/auth/`：JWT 鉴权、Bcrypt 加密与内网免密策略中间件。
- `internal/backup/`：数据库备份与一键恢复引擎。
- `internal/migrate/`：AutoBangumi 及历史数据迁移导入器。

## 🎨 前端目录 (`web/`)
基于 Vue 3.5 + TypeScript + Vite + TailwindCSS 构建的响应式前端：
- `web/src/views/`：业务视图页面（订阅、时间表、搜索、下载、设置、插件、日志看板）。
- `web/src/components/`：通用 UI 与卡片组件。
- `web/src/composables/`：组合式函数（`useVersion`, `useToast`, `useWebSocket` 等）。
- `web/src/locales/`：中英文双语国际化词条字典。
- `web/package.json`：前端依赖配置（Node 24+）。
- *注：生产构建产物输出至 `web/dist`，由 `embed.go` 直接内嵌至后端二进制中。*

## 📁 标准文档体系 (`docs/`)
- `docs/00_DESIGN_PHILOSOPHY.md`：微内核宪法与解耦哲学。
- `docs/01_ARCHITECTURE.md`：系统架构白皮书。
- `docs/02_PLUGIN_SPEC.md`：插件开发规范与接口白皮书。
- `docs/03_MCP_SPEC.md`：MCP 协议服务端与智能体交互白皮书。
- `docs/04_AUTOPARSER_SPEC.md`：全自动三级消歧引擎白皮书。
- `docs/05_DEPLOYMENT.md`：生产环境部署与容器化实践指南。
- `docs/API.md`：RESTful API 与 MCP 接口全量规范文档。
- `docs/DEVELOPMENT_PLAN.md`：功能规划与阶段交付清单（双语）。
- `docs/PROJECT_CONTEXT.md`：项目核心背景与技术约束备忘录（双语）。
- `docs/TRANSFER_CONTEXT.md`：多环境工作流迁移上下文（双语）。