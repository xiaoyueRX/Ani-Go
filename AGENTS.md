# AGENTS.md

此文件为 AI 助手在处理本仓库代码时提供指导。

## 技术栈与架构
- **语言**: Go 1.25+
- **数据库**: SQLite via GORM (`github.com/glebarez/sqlite` - 纯 Go 驱动，无 CGO 依赖)
- **关键依赖**: `golang-jwt/jwt/v5`（JWT 鉴权）、`goquery`（HTML 解析）、`golang.org/x/crypto`（Bcrypt）
- **架构**: 接口驱动 (Hexagonal/Clean Architecture)
- **核心接口**: 定义在 `internal/core/interfaces.go` (Source, Downloader, MetadataProvider, Organizer, Notifier, EventBus)
- **前端**: Vue3 + Vite + TypeScript + TailwindCSS v4 + DaisyUI v5（`web/` 目录）

## 常用命令
- **运行**: `go run .`（在项目根目录执行）
- **构建**: `go build ./...`
- **测试**: `go test ./... -v`
- **单测**: `go test -v -run TestName ./internal/package/`
- **依赖管理**: `go mod tidy`
- **前端开发**: `cd web && npm run dev`
- **前端构建**: `cd web && npm run build`
- **GFW 环境 Go 代理**: `GOPROXY=https://goproxy.cn,direct go get ./...`

## 项目约定与注意事项

### 接口优先
新功能（新下载器、新资源站）必须实现 `internal/core/interfaces.go` 中对应接口，不得直接修改核心逻辑。主程序只依赖接口不依赖实现。

### 数据库驱动
必须使用 `github.com/glebarez/sqlite`（纯 Go 驱动），禁止使用 `gorm.io/driver/sqlite`（需要 CGO，会破坏跨平台编译）。

### 配置管理
通过 `internal/config/config.go` 加载，环境变量优先级高于默认值。设置项存储在数据库 `settings` 表中，通过 `/api/settings` 管理。

### 敏感信息
Token、密码等严禁硬编码（例如 `MIKAN_RSS_URL`, `QB_PASS`, `TMDB_API_KEY`, `BGMTV_USER_TOKEN`）。始终使用环境变量注入。

### 文件编码
所有源文件必须是无 BOM 的 UTF-8 编码。避免使用 PowerShell 默认的 `>` 重定向。

### 文档规范
- **Go 源码注释**: 中文
- **总结性文档**: 中英双语各一份（如 `README.md` + `README_EN.md`）
- **CLAUDE.md**: 中文，用于 Claude Code 上下文

### GFW 网络环境
GitHub、Go 代理、Mikan 等境外服务可能被墙：
- Go 模块：`GOPROXY=https://goproxy.cn,direct`
- Mikan：内置 `proxyDomain` + `mirrorDomains` 多域名自动回退
- BGM.tv：`api.bgm.tv` → `api.bangumi.tv` → `api.chii.in` 依次尝试
- TMDB：通过 `TMDB_MIRROR_DOMAINS` 配置镜像

### 路径处理
注意 Docker 容器内路径与宿主机路径的映射关系（例如容器内 `/TV` 对应宿主机 `/vol2/1000/TV`）。

### API 设计
- Go 1.22+ `http.ServeMux` 方法路由（`GET /path`, `POST /path`）
- JWT Bearer Token 鉴权（`crypto/rand` 动态密钥，每次重启重新生成）
- 所有 `/api/*` 路径受 AuthMiddleware 保护（`/api/login` 除外）
- 请求/响应格式：JSON（`Content-Type: application/json; charset=utf-8`）

### 事件总线
使用 EventBus 进行组件间松耦合适信（如 `download.completed` → 触发文件整理 → `file.organized`）。

### GORM 注意事项
- `default:true` 标签会导致零值 `false` 被覆盖，更新 bool 字段需用 `db.Model().Update("field", false)`
- 软删除通过 `gorm.Model` 的 `DeletedAt` 字段实现
- 级联删除需手动处理（如删除订阅时同时删除关联剧集）

## 当前项目状态
- **Phase 0** ✅ — 项目初始化
- **Phase 1** ✅ — 核心引擎 MVP
- **Phase 2** ✅ — 历史补全 + 元数据 + 镜像支持 + 死种超时告警 + 自定义正则
- **Phase 3** ✅ — Web UI + RESTful API + Docker + CI/CD
- **Phase 4** ✅ — AI 多协议 + qBittorrent/Transmission/Aria2 + 插件系统 + 多资源站
- **Phase 5** ✅ — 16 平台消息通知 + 自然语言任务解析器
- **Phase 6** ✅ — 数据迁移工具（AutoBangumi 导入）
- **测试**：108 个测试全通过

## 关键文件速查

| 文件 | 作用 |
|------|------|
| `internal/core/interfaces.go` | 7 核心接口 + 事件常量 + 数据类型定义 |
| `internal/config/config.go` | 配置结构体 + env 加载 + 默认值 + DB 回退 |
| `internal/database/models.go` | 5 个 ORM 模型 |
| `internal/api/server.go` | HTTP 路由注册 + 中间件链 + 服务生命周期 |
| `internal/api/handlers.go` | API 处理器（订阅 CRUD、下载、设置、解析、迁移） |
| `internal/source/mikan.go` | Mikan RSS 解析 + HTML 详情页爬取 + 镜像回退 |
| `internal/source/multi.go` | 多资源站聚合器（Nyaa/ACGRIP/AnimeTosho） |
| `internal/scheduler/scheduler.go` | RSS 轮询 + 文件整理 + 补全扫描 + TriggerSupplement |
| `internal/downloader/qbittorrent.go` | qBittorrent Web API 客户端 |
| `internal/downloader/transmission.go` | Transmission RPC 客户端 |
| `internal/downloader/aria2.go` | Aria2 JSON-RPC 客户端 |
| `internal/metadata/tmdb.go` | TMDB API v3 元数据提供者 |
| `internal/metadata/bangumi.go` | BGM.tv 元数据提供者 |
| `internal/notifier/` | 16 平台通知实现（Telegram/Discord/QQ/LINE/WhatsApp 等） |
| `internal/ai/` | AI 4 协议适配（OpenAI/Google/Anthropic/Ollama） |
| `internal/parser/` | 自然语言任务解析器（正则 + AI 回退） |
| `internal/plugin/` | 插件系统（Webhook + Shell 脚本） |
| `main.go` | 启动流程：Config → JWT → DB → EventBus → Source → Downloader → Organizer → Scheduler → API |
