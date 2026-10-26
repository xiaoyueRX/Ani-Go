# Ani-Go

自托管的自动追番与媒体整理工具。

Ani-Go 根据订阅源跟踪番剧更新，将资源交给已有下载器，并按命名规则整理到媒体库。适合部署在 NAS 或家庭服务器上。

[English](README_EN.md) · [版本发布](https://github.com/xiaoyueRX/Ani-Go/releases) · [问题反馈](https://github.com/xiaoyueRX/Ani-Go/issues) · [部署说明](docs/05_DEPLOYMENT.md)

![CI](https://github.com/xiaoyueRX/Ani-Go/actions/workflows/ci.yml/badge.svg)

## 能做什么

- **追番与检索**：使用蜜柑计划 RSS 跟踪更新；可启用 Nyaa、ACG.RIP、AnimeTosho 等资源源进行聚合搜索。
- **下载管理**：连接 qBittorrent、Transmission 或 Aria2，提交任务、查看状态，并在设置中切换当前下载器。
- **季集识别**：结合标题规则、放送时间等信息识别剧集；AI 辅助解析为可选能力，需要自行配置服务。
- **媒体整理**：按模板生成番剧、电影和特别篇目录；支持硬链接，并在无法硬链接时回退为复制。
- **管理界面**：通过 Web 页面管理订阅、下载、放送日历和系统设置，支持中英文界面。
- **其他功能**：通知推送、内置插件、备份以及通过 MCP 接口进行部分远程管理。

Ani-Go 本身不是 BT 客户端，也不是媒体播放器。实际下载依赖外部下载器；整理后的文件可交给 Jellyfin、Emby 等媒体服务器使用。

## 快速开始

### Docker Compose

需要 Docker 和 Docker Compose，以及至少一个可用的下载器。

```bash
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go
cp .env.example .env
```

启动前请完成以下配置：

1. 在 `.env` 中填写订阅源和下载器的连接信息。
2. 将 `docker-compose.yml` 中的 `/path/to/your/data:/data` 改成真实的数据目录。
3. 确认下载器能访问相同的下载文件，且 Ani-Go 配置的路径与实际挂载一致。

```bash
docker compose up -d
```

打开 `http://localhost:20001`，使用初始账号 `admin` / `admin` 登录，**立即修改密码**。

> Compose 示例把服务端口映射到宿主机的 `20001`，并将数据库和媒体目录放在 `/data` 下。容器里的 `localhost` 指容器自身；如果下载器运行在其他容器或主机，请填写它实际可访问的地址。

### 独立二进制

从 [Releases](https://github.com/xiaoyueRX/Ani-Go/releases) 下载对应平台的发行包，解压后按需配置 `.env` 并运行：

```bash
./anigo
```

Windows 环境可运行 `anigo.exe`。独立部署时请确认进程对数据库目录、下载目录和媒体目录具有读写权限。

### 从源码构建

需要 **Go 1.25+** 和 **Node.js 24 + npm**。先构建前端，再构建后端，以便将前端静态资源嵌入 Go 程序。

```bash
cd web
npm ci
npm run build
cd ..

go build -o anigo .
./anigo
```

## 基本配置

配置示例见 [`.env.example`](.env.example)。主要参数如下：

| 配置项 | 作用 |
| --- | --- |
| `PORT` | Web 与 API 监听端口，默认 `20001` |
| `MIKAN_RSS_URL` | 蜜柑计划个人 RSS 地址 |
| `DEFAULT_DOWNLOADER` | 当前使用的下载器：`qbittorrent`、`transmission` 或 `aria2` |
| `QB_HOST` / `QB_USER` / `QB_PASS` | qBittorrent Web API 地址与凭证 |
| `TR_HOST` / `TR_USER` / `TR_PASS` | Transmission RPC 地址与凭证 |
| `ARIA2_HOST` / `ARIA2_SECRET` | Aria2 RPC 地址与密钥 |
| `TV_BASE_PATH` | 番剧整理目录 |
| `MOVIE_BASE_PATH` / `OVA_BASE_PATH` | 电影与特别篇整理目录 |
| `USE_HARDLINK` | 是否优先使用硬链接 |
| `DB_PATH` | SQLite 数据库文件路径 |

配置值可能来自环境变量、数据库中的设置或程序默认值。**环境变量优先级最高**；Compose 文件中直接写入的 `environment` 也会覆盖 `.env` 中的同名变量。修改前建议先核对 [Compose 示例](docker-compose.yml)。

### 下载目录与硬链接

硬链接要求源文件与目标文件位于同一文件系统。Docker 部署时，建议统一挂载数据父目录，例如：

```yaml
volumes:
  - /volume1/data:/data
```

将下载目录和媒体目录分别安排在 `/data/downloads` 与 `/data/media` 下，并确保下载器和 Ani-Go 都能访问对应文件。跨文件系统或权限不足时，硬链接无法创建，程序可能回退到复制；这会额外占用磁盘空间。

更具体的路径及权限说明参见 [NAS 部署文档](docs/05_DEPLOYMENT.md)。

## 安全与使用注意

- 首次登录后立刻修改默认管理员密码。
- 不要将 `20001` 端口直接暴露到公网；远程访问优先使用 VPN，或配置 HTTPS 反向代理及额外访问控制。
- 请勿提交包含密码、RSS Token 或 API Key 的 `.env`、数据库或备份文件。
- MCP 接口提供管理能力，启用远程访问时应使用独立且足够强的凭证。
- 下载器、数据源和媒体服务由用户自行配置；资源可用性及下载速度不受 Ani-Go 保证。使用时请遵守所在地法律及相关服务条款。

## 开发与文档

后端使用 Go、GORM 和 SQLite；前端使用 Vue 3、TypeScript、Vite 和 Tailwind CSS。

```bash
go test ./...
go test -race ./...
cd web && npm ci && npm run build
```

进一步阅读：

- [系统架构](docs/01_ARCHITECTURE.md)
- [API 文档](docs/API.md)
- [插件说明](docs/02_PLUGIN_SPEC.md)
- [MCP 接口说明](docs/03_MCP_SPEC.md)
- [开发计划](docs/DEVELOPMENT_PLAN.md)

欢迎通过 [Issues](https://github.com/xiaoyueRX/Ani-Go/issues) 报告问题或提出建议，也可以通过 Pull Request 提交改进。115 云下载目前尚未内置，相关需求在 [Issue #1](https://github.com/xiaoyueRX/Ani-Go/issues/1) 讨论中。

**许可证说明：** 当前仓库尚未包含 `LICENSE` 文件，因此暂不在 README 中声明特定开源许可证；授权条款需由维护者补充明确。
