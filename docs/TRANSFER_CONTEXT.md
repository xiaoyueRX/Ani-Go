# Ani-Go 项目迁移上下文

> 此文件用于从 Windows 开发机迁移到家中服务器 PVE LXC 云端 VS Code 后快速恢复工作上下文。

## 快速恢复（3 步）

```bash
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go
cp .env.example .env
# 编辑 .env 填入实际配置
```

## 当前状态

- **分支**: main
- **Phase 0**: 全部完成 ✅
- **Phase 1（核心引擎 MVP）**: 全部完成 ✅ — 31 个测试通过，Mikan RSS 解析器、qBittorrent 客户端、调度器、文件整理、EventBus、8 种标题正则解析模式
- **Phase 2（历史补全 + 元数据）**: 全部完成 ✅ — Mikan HTML 全页爬取（goquery）、TMDB/BGM.tv 元数据提供者、GFW 镜像/代理回退机制、补全调度器
- **Phase 3（Web UI + RESTful API）**: 进行中 — Vue3 前端（登录 + 仪表盘）、RESTful API（订阅 CRUD、下载队列、设置管理）
- **测试**: 63 个测试全通过

## 技术栈

- Go 1.25+
- SQLite（纯 Go 驱动 `github.com/glebarez/sqlite`，无 CGO）
- GORM ORM
- goquery（HTML 解析，Go 版 Jsoup）
- 架构：接口驱动（Source / Downloader / MetadataProvider / Organizer / Notifier / EventBus）
- 前端：Vue3 + Vite + TypeScript + TailwindCSS v4 + DaisyUI v5
- JWT 鉴权（`golang-jwt/jwt/v5` HS256）+ Bcrypt 密码哈希

## 关键文件

| 文件 | 说明 |
|------|------|
| `main.go` | 入口，打印 banner，加载配置，初始化所有模块 |
| `internal/core/interfaces.go` | 6 个核心接口 + 12 个事件常量 |
| `internal/config/config.go` | 配置加载（环境变量优先） |
| `internal/database/db.go` | GORM 初始化 + AutoMigrate |
| `internal/database/models.go` | 5 个 ORM 模型（Subscription, Episode, DownloadRecord, Setting, User） |
| `internal/source/mikan.go` | Mikan RSS 解析 + HTML 详情页全量爬取 |
| `internal/scheduler/scheduler.go` | 定时任务：RSS 轮询、文件整理、补全扫描 |
| `internal/api/server.go` | HTTP API 服务 + JWT 鉴权中间件 |
| `internal/api/handlers.go` | RESTful API 处理器：订阅 CRUD、下载队列、设置 |
| `internal/metadata/tmdb.go` | TMDB API v3 元数据提供者 |
| `internal/metadata/bangumi.go` | BGM.tv 元数据提供者 |
| `internal/downloader/qbittorrent.go` | qBittorrent Web API 客户端 |
| `AGENTS.md` / `AGENTS_EN.md` | AI 助手指南（中/英） |
| `CLAUDE.md` | Claude Code 指导文件 |
| `docs/DEVELOPMENT_PLAN.md` | 完整 5 阶段开发日程（中/英） |
| `docs/PROJECT_CONTEXT.md` | 项目核心记忆（中/英） |

## 关键约束

- Token/密码严禁硬编码，必须环境变量注入
- 文件编码 UTF-8 无 BOM
- 数据库驱动必须用 `github.com/glebarez/sqlite`
- Go 源码注释用中文，文档中英双语
- 新功能必须实现对应接口
- GFW 环境：使用 `GOPROXY=https://goproxy.cn,direct`，Mikan/BGM/TMDB 均已配置多镜像域名自动回退

## 服务器环境

- PVE 9.1 → fnOS Docker 宿主机
- 存储：`/vol2/1000/TV/Media/番剧`（TV）/ `/vol2/1000/TV/Media/剧场版`（Movie）
- 网络：Lucky v2.27.2 反向代理，HTTPS 端口 16601/50929
- 禁止跨子网 LAN 广播，需经 Lucky 反代或内网隧道
