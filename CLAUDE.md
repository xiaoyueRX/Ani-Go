# CLAUDE.md

此文件为 Claude Code (claude.ai/code) 处理本仓库代码时提供指导。

---

## 【绝对指令】核心纪律

1. **语言强制**：100% 使用简体中文交流。Go 源码注释、Git 提交信息、文档更新必须为纯中文。仅代码标识符（变量名、函数名）使用英文。
2. **CGO 红线**：绝对禁止引入 CGO 依赖（如 `go-sqlite3`、`mattn/go-sqlite3`），强制使用纯 Go 驱动 `github.com/glebarez/sqlite`。`CGO_ENABLED=0` 必须始终可用。
3. **记忆持久化**：每次完成 Feature、Bugfix 或 Phase 任务后，必须主动更新本文件的"当前项目状态"章节，并同步更新项目中所有 .md 文件（CLAUDE.md、README、AGENTS、DEVELOPMENT_PLAN、TRANSFER_CONTEXT、PROJECT_CONTEXT 及其中英文副本），确保文档与代码状态绝对一致。
4. **文档双语**：所有总结性文档（README、AGENTS、DEVELOPMENT_PLAN、TRANSFER_CONTEXT、PROJECT_CONTEXT）必须生成纯中文和纯英文两份，中文为主。文件名格式：`XXX.md`（中文）+ `XXX_EN.md`（英文）。
5. **接口优先**：新功能必须先定义接口再实现，核心模块全部抽象为接口，主程序只依赖接口不依赖实现。
6. **敏感信息隔离**：Token、密码、密钥严禁硬编码，必须通过环境变量或 `.env` 注入。

---

## 常用命令

```bash
# 构建与运行（Go 后端）
go run .                        # 运行服务（前端已 //go:embed 嵌入）
go build ./...                  # 编译所有包
go test ./... -v                # 运行所有测试
go test -v -run TestHash ./internal/auth/  # 单个测试

# 前端开发（web/）
cd web && npm install           # 安装依赖（首次）
cd web && npm run dev           # 开发服务器（Vite 热更新）
cd web && npm run build         # 生产构建（输出到 web/dist/）

# 依赖
go mod tidy                     # 同步 Go 依赖

# Docker 构建
docker build -t ani-go .
docker compose up -d
```

## 架构

**接口驱动设计**。所有核心模块抽象为 `internal/core/interfaces.go` 中定义的接口：`Source`、`Downloader`、`MetadataProvider`、`Organizer`、`Notifier`、`EventBus`。添加新下载器（如 Transmission）或资源站只需实现对应接口，不得修改现有核心逻辑。

**启动流程**（`main.go`）：配置加载 → JWT 动态密钥（crypto/rand 32B，每次重启重新生成）→ 数据库初始化 + 默认管理员（Bcrypt）→ EventBus → Mikan 资源源 → qBittorrent 下载器 → 整理器 → 调度器（RSS 轮询）→ HTTP 服务（含嵌入式前端 + 优雅关闭）。

**API 路由**：Go 1.22+ `http.ServeMux` 方法路由（`GET /api/subscriptions`、`POST /api/subscriptions`）。所有 `/api/*` 路径受 JWT AuthMiddleware 保护（`/api/login` 除外）。非 `/api/*` 路径由嵌入的 `web/dist` 静态文件服务接管，SPA 路由回退到 `index.html`。

**前端嵌合**：`web/dist` 通过 `//go:embed` 嵌入到 Go 二进制，单文件部署。Vue3 Router History 模式的 404 回退由自定义 `staticHandler` 处理。

## 关键约定

- **Windows 开发 / Linux 部署**：开发者在 Windows (PowerShell) 开发，部署到 PVE LXC 容器 (Debian)。代码必须双平台编译通过。
- **GFW 网络环境**：Go 模块用 `GOPROXY=https://goproxy.cn,direct`。Mikan、BGM.tv、TMDB API 已内置多域名镜像自动回退（`tryMirrors`）。
- **JWT 鉴权**：动态密钥由 `crypto/rand` 生成，绝不硬编码。users 表存储 Bcrypt 哈希。`/api/me` 端点在 AuthMiddleware 校验后再次验证 Token（双重保险）。
- **中间件链**：`ProxyHeadersMiddleware`（Lucky v2.27.2 反向代理 X-Forwarded-* 兼容）→ `CORSMiddleware` → `AuthMiddleware`（放行 `/api/login` 和 `/api/health`，保护其余 `/api/*`）。
- **GitHub Push**：GFW 阻断 GitHub 直连，需 VPN（TUN 模式全局代理）方可 push。

## 通知系统（`internal/notifier/`）

EventBus 驱动，`Notifier` 接口统一。已实现 16 个平台 + `MultiNotifier` 聚合广播：

| 平台 | 实现文件 | 方式 | 环境变量 |
|------|---------|------|---------|
| Telegram | `telegram.go` | Bot API + Markdown | `TELEGRAM_BOT_TOKEN` / `TELEGRAM_CHAT_ID` |
| Discord | `webhook.go` | Webhook | `DISCORD_WEBHOOK` |
| 企业微信 | `webhook.go` | Webhook bot | `WECOM_WEBHOOK` |
| 飞书 | `webhook.go` | Webhook bot | `FEISHU_WEBHOOK` |
| 钉钉 | `webhook.go` | Webhook bot | `DINGTALK_WEBHOOK` |
| QQ | `onebot.go` | OneBot HTTP API (NapCat/go-cqhttp/Lagrange) | `ONEBOT_HOST` / `ONEBOT_TOKEN` / `ONEBOT_USER_ID` / `ONEBOT_GROUP_ID` |
| Slack | `slack.go` | Webhook + Block Kit | `SLACK_WEBHOOK` |
| Matrix | `matrix.go` | Client-Server API / PUT message | `MATRIX_HOMESERVER` / `MATRIX_TOKEN` / `MATRIX_ROOM_ID` |
| LINE | `line.go` | Messaging API / push message | `LINE_CHANNEL_TOKEN` / `LINE_USER_ID` |
| WhatsApp | `whatsapp.go` | Meta Cloud API (graph.facebook.com) | `WHATSAPP_PHONE_ID` / `WHATSAPP_TOKEN` / `WHATSAPP_TO` |
| ServerChan | `push.go` | HTTP push | `SERVERCHAN_KEY` |
| Bark (iOS) | `push.go` | HTTP push | `BARK_DEVICE_KEY` |
| Pushover | `push.go` | HTTP push | `PUSHOVER_TOKEN` / `PUSHOVER_USER` |
| Gotify | `push.go` | HTTP push | `GOTIFY_URL` / `GOTIFY_TOKEN` |
| ntfy | `push.go` | HTTP push | `NTFY_URL` |
| Email | `email.go` | SMTP (context 超时) | `EMAIL_SMTP_HOST` / `EMAIL_USERNAME` / `EMAIL_TO` 等 |

自动订阅事件：`download.started/completed/failed` + `supplement.completed`。

## AI 辅助模块（`internal/ai/`）

支持 4 种协议，接口 `Classifier` 统一抽象。`NewClient()` 自动检测端点类型，`NewClientWithProtocol()` 可显式指定。

| 协议 | 端点特征 | 认证方式 | 默认模型 |
|------|---------|---------|---------|
| OpenAI | `/v1/chat/completions` | `Bearer` Header | `gpt-4o-mini` |
| Google | `generativelanguage.googleapis.com` | `?key=` Query | `gemini-2.0-flash` |
| Anthropic | `api.anthropic.com/v1/messages` | `x-api-key` Header | `claude-haiku-4-5-20251001` |
| Ollama | `:11434/api/chat` | 无 | `llama3` |

环境变量：`AI_PROTOCOL` (openai/google/anthropic/ollama/auto)、`GEMINI_API_KEY`、`CLAUDE_API_KEY`、`OLLAMA_HOST`。

## 任务解析器（`internal/parser/`）

自然语言 → 结构化任务。用户输入 "追番 某科学的超电磁炮 第一季 1080p" 自动解析为订阅参数。

- `RegexParser`：正则提取（动作词、季号、分辨率、标题），支持中英文和 S01/Season 1/第一季 等多种格式
- `AIParser`：AI 回退，调用 `ai.Classifier.Chat()` 发送自定义 system prompt
- `CompositeParser`：先正则后 AI，置信度 < 0.4 时启用 AI 回退

API 端点：`POST /api/parse` → `{"input": "追番 XXX"}` → 返回 `core.ParseResult`（action, title, season, resolution, subgroup_pref, keywords, confidence）

## Web 前端（`web/`）

Vue3 + Vite + TypeScript + TailwindCSS v4 + DaisyUI v5。前端已通过 `//go:embed web/dist` 嵌入二进制，无需额外部署。Router guard（`beforeEach`）检查 localStorage JWT Token；Axios 拦截器注入 `Authorization: Bearer <token>` 并在 401 时重定向到 `/login`。`index.html` 设置 `data-theme="dark"` 启用 DaisyUI 暗色模式。

**Node 24 ESM 兼容**：CSS 中 `@plugin "daisyui"` 在 Node 24 下失败，改用 `@import "daisyui/daisyui.css"`。

## Mikan 标题解析

`internal/source/mikan.go` 中的 `ParseMikanTitle()` 是正则最密集的模块。8 种正则模式覆盖：`SxxExx`、短横线集数、Vol、中文第X話、EPxx、#xx、【xx】、[xxv2]。特殊处理：`.5` 半集、合集检测、中文数字转换（一→1、二十→20）。输出：字幕组、标题、季、集数、分辨率、版本、标志位（合集/特别篇）。

## Mikan HTML 爬取

`parseMikanDetailHTML()` 使用 goquery 解析 Mikan 番剧详情页：
- 字幕组：`.leftbar-item a.subgroup-name`（`data-anchor` 属性）
- 种子表格：`a[name="{id}"]` → `NextAllFiltered("table")` → `tbody tr`
- 磁力链接：`a[data-clipboard-text]`（正则提取 40 位 hex hash）
- 文件大小：`parseSize()`（"1.2 GB"、"500 MB" → bytes）
- `tryMirrors()`：proxyDomain → 主域名 → mirrorDomains 依次尝试，返回首个 200

## API 端点

| 方法 | 路径 | 说明 |
|--------|------|---------|
| `POST` | `/api/login` | 登录，返回 JWT Token |
| `GET` | `/api/me` | 当前用户信息 |
| `GET` | `/api/health` | 健康检查（无需鉴权） |
| `GET` | `/api/subscriptions` | 订阅列表（?enabled=&completed=） |
| `POST` | `/api/subscriptions` | 创建订阅 |
| `GET` | `/api/subscriptions/{id}` | 订阅详情 + 剧集列表 |
| `PUT` | `/api/subscriptions/{id}` | 部分更新（指针字段） |
| `DELETE` | `/api/subscriptions/{id}` | 删除订阅 + 级联删除剧集 |
| `POST` | `/api/subscriptions/{id}/trigger-supplement` | 手动触发历史补全 |
| `GET` | `/api/downloads` | qBittorrent 下载队列 |
| `GET` | `/api/settings` | 所有设置（键值对） |
| `PUT` | `/api/settings` | 批量更新设置 |
| `GET` | `/api/settings/custom-regex` | 自定义正则规则 |
| `POST` | `/api/settings/custom-regex/reload` | 重载自定义正则 |
| `GET` | `/api/plugins` | 已加载插件列表 |
| `POST` | `/api/plugins/reload` | 重载插件配置 |
| `POST` | `/api/migrate` | 导入 AutoBangumi 数据 |
| `POST` | `/api/parse` | 自然语言解析任务（追番/订阅等） |

## GORM 注意事项

- `default:true` 标签导致零值 `false` 在 Create 时被覆盖。对 `Enabled: false` 的种子数据，Create 后用 `db.Model().Update("enabled", false)`。
- 软删除通过 `gorm.Model.DeletedAt` 实现，记录隐藏而非移除。
- 级联删除需手动处理（如删除订阅时需同时删除关联剧集）。

## 当前项目状态

- **Phase 0** ✅ — 项目初始化与架构搭建
- **Phase 1** ✅ — 核心引擎 MVP（Mikan RSS + qBittorrent + 调度器 + 整理器 + EventBus）
- **Phase 2** ✅ — 历史补全 + TMDB/BGM.tv 元数据 + GFW 镜像回退 + 补全调度器 + 死种超时告警 + 用户自定义正则
- **Phase 3** ✅ — Web UI（Vue3 订阅管理/下载队列/设置）+ RESTful API（9 端点）+ go:embed 前端嵌合 + Docker 多阶段构建 + CI/CD（GitHub Actions 多架构镜像）
- **Phase 4** ✅ — qBittorrent/Transmission/Aria2 多下载器 + AI 多协议辅助（OpenAI/Google/Anthropic/Ollama） + 插件系统（Webhook/脚本）+ 死种超时检测 + 用户自定义正则 + 多资源站（Nyaa/ACG.RIP/AnimeTosho + MultiSource 聚合器）
- **Phase 5** ✅ — 16 平台消息通知（Telegram/Discord/WeCom/Feishu/DingTalk/QQ/Slack/Matrix/LINE/WhatsApp/ServerChan/Bark/Pushover/Gotify/ntfy/Email + MultiNotifier 聚合广播 + EventBus 自动推送）
- **Phase 6** ✅ — 数据迁移工具（AutoBangumi SQLite 导入）+ 额外通知平台补全
- **测试**：108 个测试全通过

详见 `docs/DEVELOPMENT_PLAN.md`。
