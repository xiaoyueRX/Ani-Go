# Ani-Go

> 全自动番剧追番下载管理系统

**Ani-Go** 是一个基于 Go 开发的开源番剧管理工具，支持自动追新番、历史全量补全、多下载器、多资源站，文件整理后可被 Jellyfin/fnOS 直接识别。

## 特性

- 🔄 **自动追番**：绑定 Mikan 个人 RSS，在 Mikan 网页订阅即自动追踪
- 📦 **历史补全**：爬取 Mikan 番剧页面，补全 RSS 覆盖不到的老集数
- ⬇️ **多下载器**：qBittorrent / Transmission / Aria2
- 🗂️ **自动整理**：重命名 + 建目录，Jellyfin 直接刮削，无需二次处理
- 🌐 **GFW 镜像回退**：Mikan / BGM.tv / TMDB 多镜像域名自动切换
- 🤖 **AI 辅助**（可选）：支持 OpenAI / Gemini / Ollama，辅助分类识别
- 🧩 **插件系统**：开放钩子，支持第三方扩展
- 🌐 **Web UI**：Vue3 + DaisyUI，浏览器管理订阅、下载队列、设置
- 🔐 **JWT 鉴权**：动态密钥 + Bcrypt，安全可靠
- 📡 **RESTful API**：完整的订阅 CRUD、下载管理、设置接口

## 快速开始

```bash
docker run -d \
  -e MIKAN_RSS_URL="https://mikanani.me/RSS/MyBangumi?token=你的token" \
  -e QB_HOST="http://qbittorrent:8080" \
  -e QB_USER="用户名" \
  -e QB_PASS="密码" \
  -v /your/tv/path:/TV \
  -p 8080:8080 \
  ghcr.io/xiaoyuerx/ani-go:latest
```

## 开发

```bash
# 克隆项目
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go

# 配置环境变量
cp .env.example .env
# 编辑 .env 填入你的配置

# 安装前端依赖（首次）
cd web && npm install && cd ..

# 构建并运行
go run .
```

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `MIKAN_RSS_URL` | Mikan 个人 RSS 地址 | - |
| `MIKAN_DOMAIN` | Mikan 主域名 | `mikanani.me` |
| `MIKAN_PROXY_DOMAIN` | Mikan 代理域名（GFW） | - |
| `MIKAN_MIRROR_DOMAINS` | Mikan 镜像域名（逗号分隔） | `mikanani.me,mikanime.tv` |
| `QB_HOST` | qBittorrent 地址 | `http://localhost:8081` |
| `QB_USER` | qBittorrent 用户名 | - |
| `QB_PASS` | qBittorrent 密码 | - |
| `TMDB_API_KEY` | TMDB API Key | - |
| `TMDB_MIRROR_DOMAINS` | TMDB 镜像域名 | - |
| `BGMTV_USER_TOKEN` | BGM.tv 用户 Token | - |
| `BGMTV_MIRROR_DOMAINS` | BGM 镜像域名 | `api.bgm.tv,api.bangumi.tv,api.chii.in` |
| `DB_PATH` | 数据库文件路径 | `/data/ani-go.db` |
| `TV_BASE_PATH` | 番剧根目录 | `/TV/Media/番剧` |
| `MOVIE_BASE_PATH` | 剧场版根目录 | `/TV/Media/剧场版` |
| `OVA_BASE_PATH` | OVA 根目录 | - |
| `PORT` | Web UI 端口 | `8080` |

## 项目结构

```
Ani-Go/
├── main.go                  # 入口
├── internal/
│   ├── api/                 # HTTP API（路由、中间件、处理器）
│   ├── auth/                # JWT 鉴权 + Bcrypt
│   ├── config/              # 配置加载（环境变量优先）
│   ├── core/                # 核心接口与类型定义
│   ├── database/            # GORM 模型 + 数据库初始化
│   ├── downloader/          # 下载器实现（qBittorrent）
│   ├── event/               # EventBus 事件总线
│   ├── metadata/            # 元数据提供者（TMDB、BGM.tv）
│   ├── organizer/           # 文件整理器
│   ├── scheduler/           # 定时任务调度器
│   └── source/              # 资源站实现（Mikan RSS + HTML 爬取）
├── web/                     # Vue3 前端
├── docs/                    # 文档（中英双语）
├── .env.example             # 环境变量模板
└── CLAUDE.md                # Claude Code 指导
```

## API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/login` | 登录获取 Token |
| `GET` | `/api/me` | 获取当前用户信息 |
| `GET` | `/api/health` | 健康检查 |
| `GET` | `/api/subscriptions` | 订阅列表 |
| `POST` | `/api/subscriptions` | 创建订阅 |
| `GET` | `/api/subscriptions/{id}` | 订阅详情 + 剧集 |
| `PUT` | `/api/subscriptions/{id}` | 更新订阅 |
| `DELETE` | `/api/subscriptions/{id}` | 删除订阅 |
| `POST` | `/api/subscriptions/{id}/trigger-supplement` | 手动触发补全 |
| `GET` | `/api/downloads` | 下载队列 |
| `GET` | `/api/settings` | 获取设置 |
| `PUT` | `/api/settings` | 更新设置 |

## 开发进度

| 阶段 | 状态 |
|------|------|
| Phase 0: 项目初始化 | ✅ 完成 |
| Phase 1: 核心引擎 MVP | ✅ 完成 |
| Phase 2: 历史补全 + 元数据 | ✅ 完成 |
| Phase 3: Web UI + RESTful API | 🚧 进行中 |
| Phase 4: AI + 多下载器 + 插件 | 📅 计划中 |
| Phase 5: 多平台消息通知 | 📅 计划中 |

## License

MIT License © xiaoyueRX
