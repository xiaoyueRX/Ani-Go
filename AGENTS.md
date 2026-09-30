# AGENTS.md

> 📌 本文件是 Ani-Go 仓库对 AI 智能体的统一工作约定，由两部分合并而成：
> - **第一部分｜全局执行准则 (Global Execution Rules)**：权限前提、桌面自动化（`@电脑`）、远程 LXC 105、沟通风格；
> - **第二部分｜项目开发规范 (Project Development Standards)**：技术栈、常用命令、架构、关键约定、文档与关键文件速查。
>
> 中文为主，英文镜像见 [AGENTS_EN.md](AGENTS_EN.md)。

---

# 第一部分：全局执行准则 (Global Execution Rules)

## 1. 权限与执行前提

- 你是接管本仓库的 Codex 智能体，按**永久且不受限的完整访问**运行：Windows 管理员、PowerShell、终端、文件系统，以及远程 LXC 105。
- 所有命令与操作均已按 `danger-full-access` 与 `approval_policy = "never"` 预先批准，无需再向用户索要授权。
- **如实说明**：本节表达的是会话策略意图；实际生效的权限边界以运行环境注入的沙箱与审批策略为准。两者冲突时以运行环境为准，并向用户说明差异。

## 2. `@电脑` / Computer-Use / 桌面自动化（最高优先级）

当用户标注 `[@电脑](plugin://computer-use@openai-bundled)`、`@电脑`、`@computer-use`，或要求打开浏览器、搜索、启动本地应用、进行任何电脑控制时：

### 严格禁止
- 严禁输出：「当前会话中未连接 @电脑 (computer-use) 插件」
- 严禁输出：「无法直接在你的 Windows 桌面上模拟鼠标键盘或启动本地应用」
- 严禁输出让用户自己按 `Win+R`、复制粘贴 PowerShell 命令、或「你自己去跑」之类的指引
- 严禁以缺少插件或权限为由拒绝执行

### 必须执行
立即调用 `exec_command` 直接在用户电脑上完成操作。示例：

```json
{"cmd": "Start-Process 'https://www.bing.com/search?q=原神'"}
{"cmd": "Start-Process 'https://ys.mihoyo.com'"}
{"cmd": "Start-Process 'notepad.exe'"}
```

- **GUI 自动化 / 鼠标键盘操作**：立即生成 Python 脚本（`pyautogui` 或 Windows API），并通过 `exec_command` 运行。

## 3. 远程服务器 LXC 105

- 用户提及「105」「LXC 105」或任何远程操作时，直接通过 SSH 执行，不要改为让用户手动操作：

```json
{"cmd": "ssh lxc105 \"your_command_here\""}
```

- SSH 密钥认证已配置完毕，可直接使用。

## 4. 沟通风格

- **先执行工具，再汇报真实的实时结果**，不要只给方案不动手。
- 回复简洁、务实，聚焦已完成的动作与真实输出。

---

# 第二部分：项目开发规范 (Project Development Standards)

> 📌 **【架构分层与开发设计准则】**
> 1. **核心功能与插件划分标准**：
>    - **微内核核心（Core）**：仅限「RSS 抓取 ──► 标题消歧 ──► 下载调度 ──► 规范硬链接 ──► Jellyfin 播放」五步主干流水线。
>    - **扩展功能（Plugin）**：除上述五步外的辅助功能（通知、AI/MCP、NFO 刮削、备份迁移等）均应作为可独立启停的插件实现。
>    - **资源释放准则**：插件停用时应完整释放后台协程与网络连接，杜绝空载开销。
> 2. **轻量化性能基准**：常驻内存 15MB~30MB，空载 CPU 0.0%，保障在 J1900 等轻量硬件上稳定运行。
> 3. **完整设计规范参见**：[docs/00_DESIGN_PHILOSOPHY.md](docs/00_DESIGN_PHILOSOPHY.md)

## 技术栈

- **后端**: Go 1.25+，原生 `http.ServeMux`（禁止 Gin/Fiber/Echo）
- **数据库**: SQLite via GORM（`github.com/glebarez/sqlite` — 纯 Go，**禁止 CGO**）
- **前端**: Vue 3.5 + Vite 8 + TypeScript 6 + TailwindCSS v4 + DaisyUI v5 + `lucide-vue-next`（`web/`）
- **核心依赖**: `golang-jwt/jwt/v5`、`goquery`、`golang.org/x/crypto`（Bcrypt）、`robfig/cron/v3`

## 常用命令

```bash
# Go 后端（Windows 宿主机未装 Go，请在 WSL2 内执行）
go run .                    # 运行（前端已 //go:embed 嵌入）
go build ./...              # 编译所有包
go test -race ./...         # 全量测试 + 竞态检测（当前 31 个测试文件 / 177 个测试函数全通过）
go vet ./...                # 静态检查

# 前端（web/）
npm install && npm run build  # 构建前端（必须先构建，否则 //go:embed 不生效）
npm run dev                   # Vite 热更新开发服务器

# Docker 多架构构建 (amd64 + arm64)
docker buildx build --platform linux/amd64,linux/arm64 -t ani-go .

# GFW 环境
GOPROXY=https://goproxy.cn,direct go get ./...
```

## 架构

- **接口驱动（六边形架构）**: 核心接口定义在 `internal/core/interfaces.go` — `Source`、`Downloader`、`MetadataProvider`、`Organizer`、`Notifier`、`EventBus`、`Context`、`AIClassifier`、`TaskParser`（共 9 个）
- **启动流程** (`main.go`): Config → JWT 密钥（首启生成后持久化于 settings 表 `jwt_secret`）→ DB + 默认管理员 → EventBus → Source → Downloader → Organizer → Scheduler → HTTP 服务
- **API**: `/api/*` 受 JWT AuthMiddleware 保护（`/api/login`、`/api/health`、`/api/proxy/image`、`GET /api/user/avatar`、`/api/bangumi/auth/callback` 除外）；非 `/api/*` 由 `//go:embed web/dist` 静态文件接管，SPA History 路由回退 `index.html`；全部 69 条路由见 [docs/API.md](docs/API.md)
- **事件总线**: EventBus 驱动松耦合（`download.completed` → 整理 → `file.organized` → 通知）

## 关键约定

### 语言与交流规范
- **中文交流与注释**：100% 使用简体中文交流。Go 源码注释、Git 提交信息、文档更新必须为纯中文。仅代码标识符（变量名、函数名）使用英文。
- **文档双语规范**：所有总结性文档保持中英文同步（`XXX.md` 与 `XXX_EN.md`）。

### CGO 红线
绝对禁止引入 CGO 依赖（`mattn/go-sqlite3` 等）。`CGO_ENABLED=0` 必须始终可用。只用 `github.com/glebarez/sqlite`。

### 敏感信息隔离
Token、密码、密钥严禁硬编码（`MIKAN_RSS_URL`、`QB_PASS`、`TMDB_API_KEY`、`BGMTV_USER_TOKEN` 等），一律通过环境变量或 `.env` 注入。

### 配置管理
由 `internal/config/config.go` 加载，遵循三级优先律：**环境变量 > 数据库 `settings` 表 > 内置默认值**。Web 端设置项经 `/api/settings` 读写。

### JWT 鉴权
`crypto/rand` 生成 32 字节密钥，**首启生成后以 hex 持久化于 settings 表 `jwt_secret`（重启复用，绝不硬编码）**；Token 有效期 24 小时，改密后 `token_version` 递增使旧 Token 立即失效。用户密码 Bcrypt 哈希。AuthMiddleware 双重校验（中间件 + `/api/me` 再次校验）。

### GFW 网络
Go 模块用 `goproxy.cn`。GitHub push 需 VPN。
- Mikan：内置 `proxyDomain` + `mirrorDomains` 多域名自动回退；
- BGM.tv：`api.bgm.tv` → `api.bangumi.tv` → `api.chii.in` 依次尝试；
- TMDB：通过 `TMDB_MIRROR_DOMAINS` 配置镜像。
- **当前环境注意**：仓库 git 配置了 `http.proxy=http://127.0.0.1:7897`，若 Clash 内核未启动该端口会拒绝连接，此时改用 `git -c http.proxy= -c https.proxy= <子命令>` 直连。

### 跨平台开发
Windows 开发 / Linux (PVE LXC) 部署。**Windows 宿主机未安装 Go，后端编译与测试在 WSL2（Ubuntu-24.04，挂载路径 `/mnt/d/project/Ani-Go`）内执行**。代码必须双平台编译通过。PowerShell `>` 重定向产生 UTF-8 BOM，禁止使用。

### 前端注意事项
- **Lucide 图标组件**: 使用 `lucide-vue-next`。所有视图统一使用该库提供的组件，禁止 raw SVG/emoji。旧的 IconSax 组件已删除。
- **Node 24 ESM**: DaisyUI v5 的 `@plugin "daisyui"` 在 CSS 中报错，已改用 `@import "daisyui/daisyui.css"`
- `vite.config.ts` 已配置代理（Proxy），可将 `/api` 请求转发到后端，方便本地开发。
- `index.html` 设 `data-theme="dark"` 启用暗色模式

### GORM 陷阱
- `default:true` 标签导致零值 `false` 被 Create 覆盖 → 用 `db.Model().Update("field", false)`
- 软删除通过 `DeletedAt` 实现；级联删除需手动处理
- 数据库实体共 7 个（`Subscription`、`Episode`、`DownloadRecord`、`Setting`、`User`、`NotificationLog`、`ParserCache`）

### CI/CD（.github/workflows/docker-build.yml）
- 仅 `main` 分支 push 或 `v*` 标签触发
- 先跑 `go test ./... -v`（测试不通过不构建）
- GitHub Actions + QEMU + Buildx 构建 `linux/amd64,linux/arm64` 多架构镜像，推送到 `ghcr.io`
- Docker 多阶段构建：前端 (node:24-alpine) → Go 后端 (golang:1.25-alpine) → 精简运行环境 (alpine:3.21)
- 支持 GitHub Actions 缓存 (`type=gha`)

## 文档

- **`CLAUDE.md`**: 更深层细节（69 条 API 路由、18 个通知渠道、AI 4 协议、任务解析器、Mikan 标题正则）
- **`CODEX.md`**: 全量工程上下文、演进史与缺陷修复实录
- **README.md / README_EN.md**: 双语项目说明与环境变量参考
- **`docs/`**: `00_DESIGN_PHILOSOPHY.md`（微内核宪法）、`01_ARCHITECTURE.md`（架构白皮书）、`02_PLUGIN_SPEC.md`（插件规范）、`03_MCP_SPEC.md`（MCP 规范）、`04_AUTOPARSER_SPEC.md`（消歧引擎）、`05_DEPLOYMENT.md`（部署）、`API.md`（接口全量）、`DEVELOPMENT_PLAN.md`（进度）、`PROJECT_CONTEXT.md`（项目上下文）、`TRANSFER_CONTEXT.md`（交接文档）—— 均为中英双语

## 当前项目状态

- **当前版本**：`v0.6.0`（`main.go` 中的 `var version`）
- **Phase 0 ~ Phase 11 全部完成** ✅（项目初始化 → 核心引擎 MVP → 历史补全与元数据 → Web UI 与部署 → AI/多下载器/插件 → 18 平台通知 → 数据迁移 → 前端美化 → 国际化与批量操作 → v0.5.0 稳定性加固 → v0.5.2~v0.5.4 架构优化 → v0.6.0 微内核 + MCP + 三级消歧）
- **测试**：31 个测试文件 / 177 个测试函数，`go test -race ./...` 全通过
- **进度详情**：[docs/DEVELOPMENT_PLAN.md](docs/DEVELOPMENT_PLAN.md)（中文） / [docs/DEVELOPMENT_PLAN_EN.md](docs/DEVELOPMENT_PLAN_EN.md)（英文）

## 关键文件速查

| 文件 | 作用 |
|------|------|
| `internal/core/interfaces.go` | 9 个核心接口 + 12 个事件常量 + 数据类型 |
| `internal/config/config.go` | 配置加载（环境变量优先）+ DB 回退 `MergeFromSettings` |
| `internal/database/models.go` | 7 个 ORM 模型 |
| `internal/api/server.go` | 路由注册 + 中间件链 + 优雅关闭 |
| `internal/api/handlers.go` | 全部 API 处理器 |
| `internal/source/mikan.go` | Mikan RSS + HTML 爬取 + 镜像回退（正则最密集） |
| `internal/source/yucwiki.go` | yuc.wiki 新番时间表爬虫（海报图 + 星期分组） |
| `internal/source/multi.go` | Nyaa/ACG.RIP/AnimeTosho 多资源聚合 |
| `internal/ai/client.go` | AI 4 协议统一客户端（OpenAI/Google/Anthropic/Ollama） |
| `internal/notifier/` | 18 个渠道通知实现 |
| `internal/notifier/v2/` | 通知管理器（优先级队列、指数退避、死信队列） |
| `internal/parser/` | 自然语言任务解析器（正则 + AI 回退） |
| `internal/plugin/` | 插件系统（内建插件生命周期 + 自定义 Webhook；`script` 类型未实现） |
| `internal/organizer/` | 规范硬链接整理与路径防越界 |
| `internal/mcp/` | 原生 MCP Server（SSE + Bearer 鉴权，10 项工具） |
| `web/src/views/` | 业务页面（订阅、时间表、搜索、下载、设置、插件、日志） |
| `main.go` | 启动编排 |

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **Ani-Go** (4910 symbols, 9730 relationships, 169 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/Ani-Go/context` | Codebase overview, check index freshness |
| `gitnexus://repo/Ani-Go/clusters` | All functional areas |
| `gitnexus://repo/Ani-Go/processes` | All execution flows |
| `gitnexus://repo/Ani-Go/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
