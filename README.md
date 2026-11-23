# Ani-Go

在 NAS 或家庭服务器上自动追番。Ani-Go 从 RSS 发现新集数，交给 qBittorrent、Transmission 或 aria2 下载，再按作品、季和集数整理到 Jellyfin、Emby 等媒体服务器可扫描的目录。Web 界面支持简体中文和英文。

[English](README_EN.md) · [部署指南](docs/05_DEPLOYMENT.md) · [发行包](https://github.com/xiaoyueRX/Ani-Go/releases/latest) · [更新记录](CHANGELOG.md) · [问题反馈](https://github.com/xiaoyueRX/Ani-Go/issues)

[![main 分支 CI](https://github.com/xiaoyueRX/Ani-Go/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/xiaoyueRX/Ani-Go/actions/workflows/ci.yml?query=branch%3Amain)

## 快速开始

### Docker Compose

准备 Docker Compose、一个可连接的下载器，以及下载器和 Ani-Go 都可读写的共享目录。

```bash
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go
cp .env.example .env
```

1. 在 NAS 上创建数据目录并给容器账户读写权限。编辑 `docker-compose.yml`，把 `source: /path/to/your/data` 改为该目录的绝对路径，`target` 保持为 `/data`。Compose 会检查宿主机目录是否存在。示例默认使用 UID/GID `1000:1000`。
2. 让下载器使用相同的容器路径。比如把原始下载存入 `/data/downloads`，把媒体服务器的剧集库指向 `/data/media/番剧`。这样保留下来做种的原文件不会被重复扫描。
3. 下载器地址、凭据和 RSS 可在首次登录后的设置中填写，也可预先填写在 `.env`。跨容器连接时用服务名或 NAS 的局域网地址，容器内的 `localhost` 指向容器自己。

```bash
docker compose up -d ani-go
docker compose logs ani-go
```

通过 `http://<NAS 的局域网地址>:20001` 登录，账户为 `admin`。当前源码在空数据库上使用 `ANIGO_ADMIN_PASSWORD` 预设首密，留空则在首次启动日志中显示随机密码。若使用 v0.7.0 发行包，请按该版本启动日志中的提示登录并更换初始密码。升级会保留已有账号。

Compose 默认拉取 GHCR 的 `latest` 镜像，跟随通过 CI 的 `main` 提交。需要固定版本时，选用与 GitHub Release 对应的版本标签。要运行当前检出的源码，可将 Compose 服务的 `image` 改为 `ani-go:local`，启用 `build: .`，运行 `docker compose up -d --build ani-go`。发行包与源码的功能差异见更新记录的「未发布」部分。[Compose 文件](docker-compose.yml) · [更新记录](CHANGELOG.md)

### 独立二进制

从 Releases 下载与操作系统和 CPU 架构匹配的压缩包，可用同一发行页的 `checksums.txt` 校验。在解压目录中配置 `.env`，运行 `./anigo`（Windows 为 `anigo.exe`）。以后从同一工作目录启动，以便继续读取配置和相对路径下的数据库。Linux 和 macOS 也可运行仓库的安装脚本下载并校验发行包。[安装脚本](install.sh) · [下载发行包](https://github.com/xiaoyueRX/Ani-Go/releases/latest)

## 从订阅到入库

1. 在设置中选定下载器，填写 Web API 地址和凭据并测试连接。配置蜜柑个人 RSS，或在订阅中指定该作品的 RSS。
2. 在番剧检索页找到作品并追番。当前源码支持从单集发布标题提取作品名、季数、字幕组和分辨率，确认后会按作品和季跟踪后续集数。蜜柑系列条目可以直接选择字幕组。
3. 在下载队列查看传输进度，在订阅详情查看每集的整理状态。订阅的下载目录留空时使用下载器自己的保存设置。整理目录可分别为番剧、电影和特别篇设定。
4. 用媒体服务器扫描整理目录。Compose 示例开启硬链接以保留下载源继续做种；源与目标在同一文件系统才能建立硬链接，跨卷时会复制文件并额外占用磁盘。

Ani-Go 还支持按历史发布补全缺集、查看放送日历、按模板命名、使用 TMDB 元数据归并系列，以及可选的 AI 解析、通知和 MCP 管理。[工作原理](docs/01_ARCHITECTURE.md) · [季集识别](docs/04_AUTOPARSER_SPEC.md) · [MCP 接入](docs/03_MCP_SPEC.md)

## 插件

官方插件由项目维护者 xiaoyueRX 维护，插件页会标明来源。其他作者的插件归入社区或第三方。可导入 Ani-Go Webhook 清单，让服务订阅事件，或声明自己的凭据、按钮和任务列表。示例中的服务地址需换成作者实际部署的 HTTPS 地址。[插件作者指南](docs/02_PLUGIN_SPEC.md) · [可导入示例](examples/plugins/cloud-tasks.json)

### 115 云下载插件

NaJue 在 issue #1 中提出「希望能提交115磁力云下载，来加速。」当前源码提供官方插件 `cloud-115`。[原始需求](https://github.com/xiaoyueRX/Ani-Go/issues/1)

1. 打开 **设置 → 插件与自动化**，启用默认关闭的 **115 磁力云下载**。
2. 保存已获用户授权且具备云下载权限的 115 开放平台 OAuth `access_token`。也可以设置环境变量 `CLOUD115_ACCESS_TOKEN`。环境令牌优先，更换或移除时请在部署环境操作。
3. 在带磁力的搜索结果选择 **发送到插件 → 115 磁力云下载** 并确认，也可以在插件卡片粘贴磁力。刷新卡片即可查看任务状态。文件存入自己的 115 云盘，需要 NAS 副本时需自行取回。本地订阅继续使用配置的下载器。

升级时，原有同名自定义 ID `cloud-115` 会得到独立 ID，作者和配置保留。启用时请在插件页核对来源。[115 授权文档](https://www.yuque.com/115yun/open) · [添加云任务](https://www.yuque.com/115yun/open/zkyfq2499gdn3mty) · [查询云任务](https://www.yuque.com/115yun/open/av2mluz7uwigz74k)

## 配置与维护

| 配置 | 常用变量 |
| --- | --- |
| Web 与存储 | `PORT` 默认 `20001`；`DB_PATH` 指定 SQLite 数据库，`BACKUP_PATH` 指定备份目录 |
| RSS | `MIKAN_RSS_URL`；`MIKAN_RSS_MODE=personal` 匹配已有订阅，`classic` 可从全局 RSS 新建订阅 |
| 下载器 | `DEFAULT_DOWNLOADER` 可选 `qbittorrent`、`transmission`、`aria2`。对应连接参数为 `QB_HOST/QB_USER/QB_PASS`、`TR_HOST/TR_USER/TR_PASS`、`ARIA2_HOST/ARIA2_SECRET` |
| 媒体库 | `TV_BASE_PATH`、`MOVIE_BASE_PATH`、`OVA_BASE_PATH` 分别控制番剧、电影和特别篇目录；`USE_HARDLINK` 控制硬链接或移动 |
| 元数据与通知 | `TMDB_API_KEY` 用于元数据检索和系列归并。环境示例包含 Telegram、微信、飞书、Signal、Bark 等渠道的参数 |

Nyaa、ACG.RIP 和 AnimeTosho 默认关闭。可在网页启用，或设置 `NYAA_ENABLED`、`ACGRIP_ENABLED`、`ANIMETOSHO_ENABLED`。对应的 `*_DOMAIN` 可替换站点域名。仅设置环境域名也会启用该源，显式开关优先。

启动配置通常优先进程环境变量，再读取支持回退的已保存设置；整理器的硬链接开关等运行时配置可由网页设置更新。Compose 的 `environment` 覆盖 `env_file` 中的同名值。[环境变量示例](.env.example) · [NAS 路径与权限](docs/05_DEPLOYMENT.md)

升级容器前，确认数据库和备份位于持久卷。当前 Compose 的默认备份目录为 `/data/backups`。重建旧容器前，检查其中的 `/app/data`。把 `/app/data/uploads` 中的头像复制到宿主机挂载目录下的 `uploads`，并取出仍需保留的备份和历史日志。对使用仓库镜像的部署，运行 `docker compose pull ani-go` 和 `docker compose up -d ani-go`，通过 `/api/health` 查看运行服务报告的版本。[升级与恢复](docs/05_DEPLOYMENT.md)

首次登录后可在设置中修改密码。远程访问建议使用 VPN 或带访问控制的 HTTPS 反向代理，MCP 使用独立凭据。请妥善保存包含 RSS 令牌、API 密钥或个人数据的配置、数据库与备份，并按来源和服务条款获取资源。

## 从源码构建

需要 Go 1.25+ 和 Node.js 24（含 npm）。Go 程序会嵌入构建好的 Vue 3 前端，以下命令从仓库根目录运行。

```bash
cd web
npm ci
npm test
npm run build
cd ..
go vet ./...
go test -race ./...
go build -o anigo .
./anigo
```

前端开发服务器、类型检查、API 和项目计划见专门文档。[前端开发](web/README.md) · [API 参考](docs/API.md) · [OpenAPI](docs/api.yaml) · [开发计划](docs/DEVELOPMENT_PLAN.md)

## 许可证

本项目以 MIT 许可证发布。第三方依赖及素材遵循各自的许可证。[LICENSE](LICENSE)
