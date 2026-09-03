# Ani-Go

> 全自动番剧追番下载管理系统
> [English Version](README_EN.md)

**Ani-Go** 是一个基于 Go 的番剧管理工具，绑定 Mikan RSS 自动追番、历史全量补全，支持多下载器与多资源站，整理后的文件可被 Jellyfin / fnOS 直接识别。

## 核心特性

- 📺 **新番时间表**：yuc.wiki 数据源，按星期分组，标准海报图
- 🔄 **自动追番 + 历史补全**：Mikan RSS 订阅即追踪，并回补老集数
- ⬇️ **多下载器**：qBittorrent / Transmission / Aria2
- 🗂️ **自动整理**：重命名 + 建目录，Jellyfin 直接刮削
- 🌐 **GFW 镜像回退**：Mikan / BGM.tv / TMDB 多镜像自动切换
- 🤖 **AI 辅助**（可选）：OpenAI / Google / Anthropic / Ollama
- 🧩 **插件系统**：开放钩子，支持第三方扩展
- 📡 **API 文档**：详见 [API 文档](./docs/API.md) 或 `http://localhost:20001/api/login`
- 🌍 **Web UI + PWA**：Vue3 管理订阅、下载队列、设置，可安装为独立应用

均支持**中英双语**，语言切换即时生效。

## 快速开始

```bash
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go
cp .env.example .env        # 填入 MIKAN_RSS_URL、QB_HOST 等
docker compose up -d
```

浏览器打开 `http://localhost:20001`，默认账号 `admin` / `admin`。

手动构建：

```bash
cd web && npm install && npm run build && cd ..
go build -o anigo .
./anigo
```

## 环境变量

完整变量见 [`.env.example`](.env.example)，核心几项：

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `MIKAN_RSS_URL` | Mikan 个人 RSS 地址 | - |
| `DEFAULT_DOWNLOADER` | 默认下载器 | `qbittorrent` |
| `QB_HOST` / `QB_USER` / `QB_PASS` | qBittorrent 连接 | `http://localhost:8081` |
| `TV_BASE_PATH` | 番剧根目录 | `./TV/番剧` |
| `PORT` | Web UI 端口 | `20001` |

## 当前版本

**v0.5.0** — 稳定性加固与插件管理闭环（下载完成通知 Panic 修复、番剧级死种超时配置、插件管理 Tab、备份恢复修复）。

## License

MIT License © xiaoyueRX
