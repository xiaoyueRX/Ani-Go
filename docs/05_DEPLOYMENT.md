# 部署到 NAS 或家庭服务器

[English](05_DEPLOYMENT_EN.md) · [快速开始](../README.md#快速开始)

## 容器部署

使用 ARM64 NAS 安装 v0.7.0 时，可选用发布附件 `ani-go-v0.7.0-linux-arm64.tar.gz`。GHCR 上 `ghcr.io/xiaoyuerx/ani-go:0.7.0` 的 ARM64 镜像内含 x86-64 程序，在原生 ARM64 主机上启动会遇到 `exec format error`。[v0.7.0 发布附件](https://github.com/xiaoyueRX/Ani-Go/releases/tag/v0.7.0)

1. 在仓库目录复制 `.env.example` 为 `.env`，填写下载器地址和凭据。容器需要通过服务名或可达的宿主机地址访问下载器。
2. 编辑仓库的 Compose 文件，把 `/path/to/your/data` 换为 NAS 上已存在的共享目录。首次部署可以使用空目录，容器账户须能在其中创建文件及子目录。默认容器以 UID/GID `1000:1000` 运行。若共享目录已授权给其他账户，可在 `.env` 填写其数字 `PUID`、`PGID`。独立使用 `docker run` 时，通过 `--user UID:GID` 选择账户。
3. 运行 `docker compose up -d ani-go`。访问 `http://localhost:20001`；从其他设备使用 NAS 的地址。首次启动可预设 `ANIGO_ADMIN_PASSWORD`，否则从 `docker compose logs ani-go` 读取随机生成的管理员密码。[Compose 文件](../docker-compose.yml) · [环境变量示例](../.env.example)

| 容器路径 | 用途 |
| --- | --- |
| `/data/db/ani-go.db` | 持久化 SQLite 数据库，包括账号、订阅、设置和插件配置。 |
| `/data/backups` | Compose 的备份目录，可通过 `BACKUP_PATH` 配置。 |
| `/data/uploads/avatars`、`/data/ani-go.log` | 上传的头像及应用日志。Compose 服务从根目录 `/` 启动。 |
| `/data/media` | Compose 将番剧、电影、特别篇分别整理到该目录下。 |
| `/data/downloads` | 可选的下载目录。下载器完成任务时报告的路径必须可由 Ani-Go 读取。 |

在 NAS 管理界面或用 `docker run` 手工创建新容器时，要沿用表中的布局，请同时挂载 `/data`、把工作目录设为 `/`，并采用 Compose 中的 `DB_PATH`、`BACKUP_PATH` 和三个媒体库路径。单独挂载 `/data` 时，官方 v0.7.0 镜像使用默认工作目录，数据库会写到容器内的 `/app/ani-go.db`。[Compose 配置](../docker-compose.yml)

订阅的**下载目录**留空时，Transmission、aria2 和 qBittorrent 使用自身的默认保存设置。需要指定目录时，请填写下载器能识别的实际路径，例如 `/data/downloads/series`。清空该字段会恢复默认值，后续新增任务才会使用新设置。整理完成的媒体另存到配置的媒体库目录。

空下载目录的订阅仍在投递前按 `DISK_MIN_FREE_GB` 检查 Ani-Go 所在机器的 `TV_BASE_PATH` 卷。显式下载目录按 Ani-Go 本机所见的路径或其已有父目录检查。远程下载器默认卷的剩余空间由下载器所在机器监控。

在群晖、fnOS 或 Unraid 上，从各自的共享目录选择统一父目录作为 `/data`。将它按相同的容器路径挂入下载器和 Ani-Go，避免下载器报告仅在另一个容器中有效的路径。媒体库和下载目录处于同一挂载、同一文件系统时可以建立硬链接。跨文件系统会额外复制一份媒体；若要保留下载源继续做种，启用 `USE_HARDLINK`。[整理实现](../internal/organizer/organizer.go)

整理前会向 qBittorrent、Transmission 或 Aria2 核对逐文件清单，只处理任务中已选、下载完成且本地大小吻合的视频。选集下载的合集中，未选文件和同目录的其他视频会留在原处。下载器无法确认文件状态时，整理会等到后续轮询。

移动模式先在媒体库所在卷准备完整文件，再提交剧集记录，最后清理下载源。数据库写入失败会保留源文件供重试。服务重启后会继续清理待移动文件；如果源路径的文件发生变化或库文件缺失，就保留源文件。如果需要下载器继续做种，请启用硬链接模式，同卷优先硬链接，跨卷复制后也会保留下载源。Linux 本地 ext4、Btrfs、XFS、F2FS 或 tmpfs 卷支持目录锁时，下次在同一目录整理会清理 Ani-Go 意外中断的暂存文件。Windows、网络共享和其他未确认可安全加锁的卷使用独立暂存目录；服务若在复制中断电，可能留下隐藏的暂存文件。[整理提交与恢复](../internal/scheduler/organizer_commit.go)

剧集默认按 `剧名 (年份)/Season 1` 整理，年份未知时使用 `剧名/Season 1`。这种写法可让 Jellyfin 单独识别剧名和年份。升级后，如果同一作品、同一年份只有旧版的 `剧名年份` 目录，后续集数和季会继续写入旧目录。可以在设置中指定自己的剧集模板。

## 备份与恢复

将 `BACKUP_PATH` 指向持久卷中可写的目录。手工和定时备份共用 `BACKUP_KEEP_COUNT`，默认保留最新 7 份。`BACKUP_CRON` 默认为每天零点，可使用五字段表达式（例如 `0 3 * * *`）或带秒的六字段表达式。在设置页保存计划或保留数后即时生效。部署时设置的同名环境变量优先。

升级前可在设置页创建并下载一份备份，按需勾选剧集。管理员 JSON 包含设置、订阅和可选的剧集记录，也可能包含插件凭据，应存放在私有目录。账号和本机登录签名密钥由持久化 SQLite 数据库保留。首个管理员只在空用户表中创建。若数据库无法读取或已有签名密钥损坏，启动会报告错误。恢复数据库访问或从本机有效的 SQLite 副本恢复密钥后可重新启动。新快照带 SHA-256 完整性校验，恢复前会核对内容。旧版生成的无摘要快照可继续恢复。跨库恢复同名同季订阅时会核对来源、字幕组及筛选条件，保留各订阅的剧集归属。恢复成功后插件会重新加载，其他启动配置可在重启后核对。[备份 API](API.md#设置与管理员备份)

## 升级与验证

先检查备份是否在持久卷上。沿用现有挂载至 `/data` 的宿主机目录。重建旧容器前，如 `/app/data/uploads` 有头像，将它复制到宿主机挂载目录中的 `uploads`；同时取出 `/app/data` 中仍需保留的旧备份和日志。执行以下操作后查看 `version` 及容器日志。

```bash
docker compose pull ani-go
docker compose up -d ani-go
curl -fsS http://localhost:20001/api/health
```

### 试用候选版和回滚

从手工创建的旧容器迁入持久卷时，先确认数据库实际位置。如果数据库在容器可写层，停止并保留旧容器，把数据库和同目录中存在的 `-wal`、`-shm` 文件一起复制到私有临时目录。从这份副本用 SQLite 的 `.backup` 创建独立快照，核对 `PRAGMA quick_check` 返回 `ok` 后，再将快照放入新空卷的 `db/ani-go.db`，授予容器账户读写权限。用原账号登录并核对订阅，再处理旧容器。

升级前记录当前镜像的仓库摘要或保留可启动的旧二进制、Compose 和环境配置。在宿主机上用 SQLite 的 `.backup` 为实际的 `DB_PATH` 创建独立快照，用 `PRAGMA quick_check` 检查快照应返回 `ok`。SQLite 的备份接口会包含已提交到 WAL 的事务。容器使用 `latest` 时，试用前将 `image` 固定为要验证的候选标签或摘要。

需要回滚时，先停止 Ani-Go，把升级后的库及其可能存在的 `-wal`、`-shm` 一起移至单独目录保存。将升级前的快照还原到原 `DB_PATH`，恢复文件所有者和权限，再用先前记录的镜像摘要或二进制启动。核对健康接口的版本、原账号登录、订阅季数及已入库剧集的路径。数据库快照只回滚应用记录，媒体和下载器中的文件需单独核对。

## 独立二进制安装

Linux 或 macOS 可使用安装脚本，它从 GitHub 下载发行包，核对同一版本的 `checksums.txt` 后安装到当前用户的目录。需要固定版本时，把示例中的 `| sh` 改成 `| ANIGO_VERSION=v0.7.0 sh`。

```sh
curl -fsSL https://raw.githubusercontent.com/xiaoyueRX/Ani-Go/main/install.sh | sh
cd "$HOME/.anigo"
"$HOME/.local/bin/anigo"
```

工作目录决定程序读取哪份 `.env`、使用哪个相对路径数据库。升级前停止正在运行的进程，重新执行安装脚本，再从同一目录启动。在仓库目录运行 `sh install.sh --uninstall` 会移除安装器管理的程序并保留配置和运行数据。Windows 或手动部署时从 Releases 下载对应压缩包，核对摘要，解压后在可写工作目录运行 `./anigo` 或 `anigo.exe`。反向代理如需启用 MCP，参照其 SSE 路径和令牌配置。[发行包](https://github.com/xiaoyueRX/Ani-Go/releases) · [MCP 接入](03_MCP_SPEC.md)
