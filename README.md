# Ani-Go

> 全自动番剧追番下载管理系统

**Ani-Go** 是一个基于 Go 开发的开源番剧管理工具，支持自动追新番、历史全量补全、多下载器、多资源站，文件整理后可被 Jellyfin/fnOS 直接识别。

## 特性

- 🔄 **自动追番**：绑定 Mikan 个人 RSS，在 Mikan 网页订阅即自动追踪
- 📦 **历史补全**：爬取 Mikan 番剧页面，补全 RSS 覆盖不到的老集数
- ⬇️ **多下载器**：qBittorrent / Transmission / Aria2
- 🗂️ **自动整理**：重命名 + 建目录，Jellyfin 直接刮削，无需二次处理
- 🤖 **AI 辅助**（可选）：支持 OpenAI / Gemini / Ollama，辅助分类识别
- 🧩 **插件系统**：开放钩子，支持第三方扩展
- 🌐 **Web UI**：浏览器管理订阅、下载队列、设置
- ⚠️ **超时警告**：智能检测死链，超时未下载完成自动提示更换字幕组

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
git clone https://github.com/xiaoyueRX/Ani-rss.git
cd Ani-rss

# 配置环境变量
cp .env.example .env
# 编辑 .env 填入你的配置

# 运行
go run .
```

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `MIKAN_RSS_URL` | Mikan 个人 RSS 地址 | - |
| `QB_HOST` | qBittorrent 地址 | `http://localhost:8081` |
| `QB_USER` | qBittorrent 用户名 | - |
| `QB_PASS` | qBittorrent 密码 | - |
| `DB_PATH` | 数据库文件路径 | `/data/ani-go.db` |
| `TV_BASE_PATH` | 番剧根目录 | `/TV/Media/番剧` |
| `PORT` | Web UI 端口 | `8080` |

## License

MIT License © xiaoyueRX
