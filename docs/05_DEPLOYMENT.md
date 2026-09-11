# Ani-Go 生产部署与 NAS 最佳实践指南 (Deployment Guide)

## 1. 存储挂载规范：规避跨卷硬链接失效

硬链接（Hardlink）是指向底层文件系统同一物理数据块的目录项，**不能跨物理分区，也不能在 Docker 中跨不同的 Volume 挂载点**。

### ❌ 常见误区（跨卷挂载导致无法硬链接，报错 Invalid cross-device link）：
```yaml
volumes:
  - /volume1/downloads:/downloads   # 挂载点 A
  - /volume1/media:/media           # 挂载点 B (跨卷导致硬链接无法建立)
```

### ✅ 推荐规范（单根卷挂载规范 Single-Volume-Mount）：
始终将下载目录与媒体库目录放在同一个父目录下，并在容器中作为**单个统一卷**挂载：
```yaml
volumes:
  - /volume1/data:/data             # 统一挂载
```
此时：
- 下载路径配置为：`/data/downloads`
- 媒体库路径配置为：`/data/media/anime`
- 数据库持久化：`/data/db/ani-go.db`
- 硬链接创建速度极快（Zero IO），既不产生磁盘复制，也不占用双倍空间。

### 🛡️ 生产环境用户权限安全规范（非 root 权限）
- 为保证宿主机与 NAS 存储安全，Ani-Go 官方容器镜像默认以非 root 用户身份（UID: `1000`, GID: `1000`，内置用户 `anigo`）运行。
- 在 Linux / 群晖 / 飞牛部署前，请确保宿主机挂载目录具有相应读写权限：
  ```bash
  # 赋予宿主机目录 1000:1000 读写权限
  sudo chown -R 1000:1000 /volume1/data
  ```

---

## 2. Docker Compose 标准部署模版

在 NAS 或服务器创建 `docker-compose.yml`：

```yaml
version: "3.8"

services:
  anigo:
    image: ghcr.io/xiaoyuerx/ani-go:latest
    container_name: anigo
    restart: unless-stopped
    # 遵循生产安全规范：非 root 用户 (UID 1000, GID 1000)
    user: "1000:1000"
    ports:
      - "20001:20001"
    environment:
      - TZ=Asia/Shanghai
      - PUID=1000
      - PGID=1000
      # 基础数据库与网络设置
      - DB_PATH=/data/db/ani-go.db
      - TV_BASE_PATH=/data/media/anime
      - MOVIE_BASE_PATH=/data/media/movie
      - OVA_BASE_PATH=/data/media/ova
      - USE_HARDLINK=true
      # 下载器关联 (以同网络下的 qBittorrent 为例，兼容 QB_USER 与 QB_USERNAME)
      - QB_HOST=http://qbittorrent:8080
      - QB_USER=admin
      - QB_PASS=adminadmin
      # 可选 MCP 服务 Token (用于 Claude/Cursor 远程连接)
      - MCP_ENABLED=true
      - MCP_AUTH_TOKEN=your-secure-token-here
    volumes:
      - /vol1/1000/anime-factory/data:/data
    networks:
      - anime-net

networks:
  anime-net:
    driver: bridge
```

---

## 3. 各平台适配要点

### 3.1 飞牛私有云 (fnOS)
- fnOS 采用基于 Debian 的底层架构，推荐通过「应用中心」->「Docker 容器」或通过 Compose 部署；
- 确保共享文件夹的权限给到运行用户（默认 PUID/PGID 1000）；
- 宿主机如果启用了科学上网代理（如端口 7890），在 Ani-Go 设置中可直接填写宿主机 LAN IP 代理地址。

### 3.2 群晖 (Synology DSM)
- 打开 Container Manager，新增项目，粘贴上述 Compose 配置；
- 在 File Station 中确认 `/volume1/data` 目录赋予 `Everyone` 或指定 Docker 用户的读写权限。

### 3.3 Unraid
- 在 Docker 页面使用 Docker Compose Manager 运行；
- 建议将 AppData 路径映射到高速 NVMe 缓存盘，媒体与下载目录映射到阵列存储盘。
