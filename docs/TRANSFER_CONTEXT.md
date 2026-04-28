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

- **分支**: main（10 次提交，最新 `e6a0cfa`）
- **Phase 0**: 全部完成 ✅
- **下一步**: Phase 1 — Mikan RSS 解析器 + qBittorrent 客户端集成

## 技术栈

- Go 1.22+
- SQLite（纯 Go 驱动 `github.com/glebarez/sqlite`，无 CGO）
- GORM ORM
- 架构：接口驱动（Source / Downloader / MetadataProvider / Organizer / Notifier / EventBus）

## 关键文件

| 文件 | 说明 |
|------|------|
| `main.go` | 入口，打印 banner，加载配置，初始化数据库 |
| `internal/core/interfaces.go` | 6 个核心接口 + 12 个事件常量 |
| `internal/config/config.go` | 配置加载（环境变量优先） |
| `internal/database/db.go` | GORM 初始化 + AutoMigrate |
| `internal/database/models.go` | 4 个 ORM 模型 |
| `AGENTS.md` / `AGENTS_EN.md` | AI 助手指南（中/英） |
| `docs/DEVELOPMENT_PLAN.md` | 完整 5 阶段开发日程（中/英） |
| `docs/PROJECT_CONTEXT.md` | 项目核心记忆（中/英） |

## Phase 1 待办（下一步）

1. Mikan RSS 解析器（实现 Source 接口）
2. qBittorrent 客户端集成（实现 Downloader 接口）
3. 基础调度器（定时轮询 RSS 并下发下载）
4. 基础文件整理（重命名 + 目录创建）

## 关键约束

- Token/密码严禁硬编码，必须环境变量注入
- 文件编码 UTF-8 无 BOM
- 数据库驱动必须用 `github.com/glebarez/sqlite`
- Go 源码注释用中文，文档中英双语
- 新功能必须实现对应接口

## 服务器环境

- PVE 9.1 → fnOS Docker 宿主机
- 存储：`/vol2/1000/TV/Media/番剧`（TV）/ `/vol2/1000/TV/Media/剧场版`（Movie）
- 网络：Lucky v2.27.2 反向代理，HTTPS 端口 16601/50929
- 禁止跨子网 LAN 广播，需经 Lucky 反代或内网隧道
