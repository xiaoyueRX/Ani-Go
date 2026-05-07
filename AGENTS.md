# AGENTS.md

## 技术栈
- **后端**: Go 1.25+, 原生 `http.ServeMux`（禁止 Gin/Fiber/Echo）
- **数据库**: SQLite via GORM (`github.com/glebarez/sqlite` — 纯 Go，**禁止 CGO**)
- **前端**: Vue3 + Vite 8 + TypeScript 6 + TailwindCSS v4 + DaisyUI v5 + Iconsax Linear (`web/`)
- **核心依赖**: `golang-jwt/jwt/v5`、`goquery`、`golang.org/x/crypto` (Bcrypt)

## 常用命令
```bash
# Go 后端
go run .                    # 运行（前端已 //go:embed 嵌入）
go build ./...              # 编译所有包
go test ./... -v            # 108 个测试全部通过

# 前端（web/）
npm install && npm run build  # 构建前端（必须先构建，否则 //go:embed 不生效）
npm run dev                  # Vite 热更新开发服务器

# Docker 多架构构建 (amd64 + arm64)
docker buildx build --platform linux/amd64,linux/arm64 -t ani-go .

# GFW 环境
GOPROXY=https://goproxy.cn,direct go get ./...
```

## 架构
- **接口驱动（六边形架构）**: 核心接口定义在 `internal/core/interfaces.go` — `Source`, `Downloader`, `MetadataProvider`, `Organizer`, `Notifier`, `EventBus`
- **启动流程** (`main.go`): Config → JWT 动态密钥 → DB + 默认管理员 → EventBus → Source → Downloader → Organizer → Scheduler → HTTP 服务
- **API**: `/api/*` 受 JWT AuthMiddleware 保护（`/api/login`、`/api/health` 除外）；非 `/api/*` 由 `//go:embed web/dist` 静态文件接管，SPA History 路由回退 `index.html`
- **事件总线**: EventBus 驱动松耦合（`download.completed` → 整理 → `file.organized` → 通知）

## 关键约定

### CGO 红线
绝对禁止引入 CGO 依赖（`mattn/go-sqlite3` 等）。`CGO_ENABLED=0` 必须始终可用。只用 `github.com/glebarez/sqlite`。

### JWT 鉴权
`crypto/rand` 动态生成 32B 密钥（重启即重生成，绝不硬编码），落盘 `data/.jwt_secret`。用户密码 Bcrypt 哈希。AuthMiddleware 双重校验（中间件 + `/api/me` 再次校验）。

### GFW 网络
Go 模块用 `goproxy.cn`。Mikan/BGM.tv/TMDB 内置多域名镜像自动回退。GitHub push 需 VPN TUN 模式。

### 跨平台开发
Windows 开发 / Linux (PVE LXC) 部署。代码必须双平台编译通过。PowerShell `>` 重定向产生 UTF-8 BOM，禁止使用。

### 前端注意事项
- **IconSax 组件**: `web/src/components/IconSax.vue`，Iconsax Linear 风格，props: `name` `size` `color`。所有视图统一使用此组件，禁止 raw SVG/emoji。
- **Node 24 ESM**: DaisyUI v5 的 `@plugin "daisyui"` 在 CSS 中报错，已改用 `@import "daisyui/daisyui.css"`
- `vite.config.ts` **未配 proxy**，开发时需 Go 后端已运行或手动配置
- `index.html` 设 `data-theme="dark"` 启用暗色模式

### GORM 陷阱
- `default:true` 标签导致零值 `false` 被 Create 覆盖 → 用 `db.Model().Update("field", false)`
- 软删除通过 `DeletedAt` 实现；级联删除需手动处理

### CI/CD（.github/workflows/docker-build.yml）
- 仅 `main` 分支 push 或 `v*` 标签触发
- 先跑 `go test ./... -v`（测试不通过不构建）
- GitHub Actions + QEMU + Buildx 构建 `linux/amd64,linux/arm64` 多架构镜像，推送到 `ghcr.io`
- Docker 多阶段构建：前端 (node:24-alpine) → Go 后端 (golang:1.25-alpine) → 精简运行环境 (alpine:3.22)
- 支持 GitHub Actions 缓存 (`type=gha`)

## 文档
- **`CLAUDE.md`**: 更深层细节（API 端点表、16 通知平台、AI 4 协议、任务解析器、Mikan 标题正则）
- **README.md / README_EN.md**: 双语项目说明与环境变量参考
- **`docs/`**: `DEVELOPMENT_PLAN.md`（进度）、`PROJECT_CONTEXT.md`（项目上下文）、`TRANSFER_CONTEXT.md`（交接文档）——中英双语

## 关键文件速查

| 文件 | 作用 |
|------|------|
| `internal/core/interfaces.go` | 7 核心接口 + 事件常量 + 数据类型 |
| `internal/config/config.go` | 配置加载（环境变量优先）+ DB 回退 |
| `internal/database/models.go` | 5 个 ORM 模型 |
| `internal/api/server.go` | 路由注册 + 中间件链 + 优雅关闭 |
| `internal/api/handlers.go` | 全部 API 处理器 |
| `internal/source/mikan.go` | Mikan RSS + HTML 爬取 + 镜像回退 + 镜像测速（854 行，正则最密集） |
| `internal/source/yucwiki.go` | yuc.wiki 新番时间表爬虫（海报图+星期分组） |
| `internal/source/multi.go` | Nyaa/ACG.RIP/AnimeTosho 多资源聚合 |
| `internal/ai/client.go` | AI 4 协议统一客户端（589 行） |
| `internal/notifier/` | 16 平台通知实现 |
| `internal/parser/` | 自然语言任务解析器（正则 + AI 回退） |
| `internal/plugin/` | Webhook + Shell 脚本插件 |
| `web/src/components/IconSax.vue` | Iconsax Linear 图标组件（20+ 图标） |
| `web/src/views/SettingsPage.vue` | 设置页（分组卡片+配置状态+密码显隐） |
| `main.go` | 启动编排（420 行） |
