# Ani-Go

<div align="center">

<p align="center">
  <strong>全自动番剧追番、下载、整理与媒体库刮削管理系统</strong><br>
  <em>Automated Anime Subscription, Download, Organization & Media Server Integration System</em>
</p>

<p align="center">
  <a href="https://github.com/xiaoyueRX/Ani-Go/releases"><img src="https://img.shields.io/github/v/release/xiaoyueRX/Ani-Go?color=blue&style=flat-square" alt="Release"></a>
  <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/Vue-3.x-4FC08D?style=flat-square&logo=vue.js" alt="Vue 3">
  <img src="https://img.shields.io/badge/Docker-Ready-2496ED?style=flat-square&logo=docker" alt="Docker">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-green?style=flat-square" alt="License"></a>
</p>

<p align="center">
  <a href="#-核心特性">核心特性</a> •
  <a href="#-快速开始">快速开始</a> •
  <a href="#-环境变量配置">配置说明</a> •
  <a href="#-插件生态">插件系统</a> •
  <a href="#-目录规范与媒体库">媒体库规范</a> •
  <a href="README_EN.md">English Version</a>
</p>

</div>

---

## 📖 项目简介

**Ani-Go** 是一款轻量、高性能且现代化的一站式番剧自动化管理系统。它基于 Go 语言后端与 Vue 3 前端构建，内存占用仅几十兆，单文件二进制内置静态资源（`go:embed`），开箱即用。

只需绑定你的 **蜜柑计划 (Mikan)** 个人 RSS，Ani-Go 即可全天候自动监控新番更新、智能回溯补全历史未下载集数，并通过 **qBittorrent / Transmission / Aria2** 调度下载。下载完成后，自动识别季数集数，以标准刮削格式（硬链接/软链接/移动）整理至媒体目录，供 **Jellyfin / Plex / Emby / fnOS** 直接刮削播放。

---

## ✨ 核心特性

### 📺 智能新番时间表 (Seasonal & Daily Schedule)
- **多数据源协同**：
  - **蜜柑计划 (Mikan Project，默认)**：官方全量新番数据源，完整支持 2000~2026 各年份与四个季度（冬/春/夏/秋）的跨季度历史回溯。
  - **Bangumi (bgm.tv) 每日放送**：原生集成 Bangumi 官方每日放送日历，且**完美联动年份与月份/季度选择器**，选择历史季度自动同步对应放送时间表。
  - **長門番堂 (yuc.wiki) 插件化**：内置可选插件，开启后可在时间表页面自由切换 Yuc.wiki 数据源。
- **纯净视觉排版**：
  - 聚焦周一至周日正片放送，彻底滤除非正片干扰。
  - 统一标准海报画幅比例（`aspect-[3/4.2]`），搭载骨架屏预加载与容错占位，消除海报错位与空白断层。
  - 一键切换「只看今日」、快速过滤与批量订阅。

### 🔄 自动追番与历史全量补齐 (Auto Tracking & Backfill)
- **零配置跟随追番**：绑定 Mikan 订阅链接，自动同步追番列表并建立监控。
- **智能历史分集全量补全**：新订阅番剧后，自动搜索发布组开播以来的所有历史分集并推入下载队列，一键追齐整季。
- **多发布组偏好配置**：支持对不同番剧绑定指定字幕组、语言过滤规则与自定义集数偏移。

### ⬇️ 全功能下载器矩阵 (Multi-Downloader Support)
- **qBittorrent**：深度原生交互，支持分类标签、自动做种管理（根据做种时间/分享率自动暂停或移除已完成任务）。
- **Transmission**：轻量级下载客户端支持。
- **Aria2**：极速 RPC 直链调度。
- **下载任务队列监控**：实时查看下载进度、速率、做种状态与错误重试。

### 🗂️ 媒体库规范自动整理 (Media Organization)
- **主流流媒体刮削标准**：生成的目录结构与文件命名严格符合 **Jellyfin / Plex / Emby / fnOS** 规范：
  ```
  TV/番剧/
  └── 葬送的芙莉莲/
      └── Season 01/
          ├── 葬送的芙莉莲 - S01E01.mp4
          └── 葬送的芙莉莲 - S01E02.mp4
  ```
- **多种文件整理模式**：
  - **硬链接 (Hardlink)**：推荐模式，零额外磁盘空间消耗，整理完毕后仍可在下载器中持续健康做种。
  - **软链接 (Symlink)** / **复制 (Copy)** / **直接移动 (Move)**。
- **智能文件名解析引擎**：内置多层级正则表达式，自动剥离字幕组前缀、分辨率标识、压制参数，智能识别正片集数、第0集与双语标题。

### 🧩 可扩展插件生态 (Plugin Ecosystem)
- **内建插件 (Built-in)**：
  - 🔗 **元数据映射器 (metadata-mapping)**：自动打通 Bangumi ID 与 TMDB / IMDB 关联。
  - 📁 **媒体库规范重命名 (standard-naming)**：规范化目录结构与文件名生成。
  - 📅 **長門番堂时间表 (yuc_schedule)**：为新番时间表提供 yuc.wiki 可选数据源拓展。
- **自定义 Webhook 扩展**：支持监听订阅新增、下载完成、文件整理就绪等核心系统事件，轻松对接第三方自动化服务（如 Telegram / 微信通知 / 自定义脚本）。

### 🌐 GFW 网络友好与多镜像智能测速 (Smart Mirror Fallback)
- 内置 Mikan、Bangumi、TMDB 国内外多镜像节点。
- 系统后台定时测速并按毫秒级延迟**自动择优切换可用镜像**，告别网络连通性困扰。

### 💻 现代化响应式设计与 PWA 客户端 (Modern UI)
- **桌面端视口锁定侧边栏**：纵向长列表浏览与向下滑动时，侧边栏始终固定在视口左侧，导航菜单与操作按钮随点随到。
- **移动端沉浸式体验**：标准贴底导航栏适配 iOS/Android 全面屏安全区域，支持独立移动端侧滑抽屉。
- **PWA 原生应用**：支持一键添加到桌面或手机主屏幕，脱离浏览器标签页作为独立轻量客户端运行。
- **中英双语国际化 (i18n)**：全界面、全设置项、全日志即时中英文无缝切换。

---

## 🚀 快速开始

### 方式一：Docker Compose（推荐，最省心）

1. 创建项目目录并拉取配置文件：
```bash
git clone --depth 1 https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go
```

2. 配置环境变量：
```bash
cp .env.example .env
# 编辑 .env，填入 MIKAN_RSS_URL、qBittorrent 连接信息与媒体库路径
vim .env
```

3. 启动容器集群：
```bash
docker compose up -d
```

4. 打开浏览器访问 `http://localhost:20001`：
   - **默认管理员账号**：`admin`
   - **默认管理员密码**：`admin`
   *(首次登录后建议立即前往「设置」修改密码)*

---

### 方式二：单文件独立运行

从 [Releases](https://github.com/xiaoyueRX/Ani-Go/releases) 下载对应操作系统的最新预编译二进制文件：

```bash
# 启动 Ani-Go（前端静态资源已完整内嵌在二进制中）
./anigo
```

浏览器打开 `http://localhost:20001` 即可开始使用。

---

### 方式三：源码手动构建

需要环境：Go 1.22+，Node.js 18+，npm

```bash
# 1. 克隆代码仓库
git clone https://github.com/xiaoyueRX/Ani-Go.git
cd Ani-Go

# 2. 构建前端
cd web
npm install
npm run build
cd ..

# 3. 编译后端（嵌入前端产物）
go build -o anigo .

# 4. 运行
./anigo
```

---

## ⚙️ 环境变量配置

完整配置说明可参见 [`.env.example`](.env.example)，核心配置项如下：

| 环境变量 | 说明 | 默认值 | 示例 / 说明 |
| :--- | :--- | :--- | :--- |
| `PORT` | Web UI 与 API 服务监听端口 | `20001` | `20001` |
| `MIKAN_RSS_URL` | 蜜柑计划个人订阅 RSS 地址 | 无 | `https://mikanani.me/RSS/MyBangumi?token=xxx` |
| `DEFAULT_DOWNLOADER`| 默认激活的下载器类型 | `qbittorrent`| `qbittorrent` / `transmission` / `aria2` |
| `QB_HOST` | qBittorrent WebUI 地址 | `http://localhost:8081` | `http://192.168.1.10:8081` |
| `QB_USER` | qBittorrent 用户名 | `admin` | `admin` |
| `QB_PASS` | qBittorrent 密码 | `adminadmin` | `你的qB密码` |
| `TV_BASE_PATH` | 番剧媒体库整理根目录 | `./TV/番剧` | 宿主机/挂载路径，如 `/media/anime` |
| `ORGANIZE_MODE` | 整理模式 | `hardlink` | `hardlink` (硬链) / `symlink` (软链) / `move` (移动) / `copy` (复制) |
| `POLL_INTERVAL` | RSS 自动轮询检测间隔 | `30m` | `15m`, `30m`, `1h` |
| `CLEANUP_ENABLED` | 是否启用做种自动清理 | `true` | `true` / `false` |
| `CLEANUP_MIN_SEED_TIME` | 种子最短做种保留时长 | `48h` | 超过此时长且达标后自动清理 |
| `CLEANUP_MIN_RATIO` | 最低分享率阈值 | `1.0` | 分享率达到 1.0 且满足做种时间后清理 |

---

## 📡 开放 API 与扩展开发

Ani-Go 提供完整的 RESTful API 与 OpenAPI 规格定义，方便二次开发与集成第三方客户端：
- **API 详细文档**：详见 [docs/API.md](./docs/API.md)
- **OpenAPI 规范**：详见 [docs/api.yaml](./docs/api.yaml)
- **Swagger / 在线调试**：服务启动后直接访问 `http://localhost:20001/api/login`

---

## 🤝 鸣谢与致敬

- [Mikan Project (蜜柑计划)](https://mikanani.me/) — 优质全面的新番 RSS 与索引源
- [Bangumi (番组计划)](https://bgm.tv/) — 二次元爱好者的精神家园与元数据库
- [長門番堂 (yuc.wiki)](https://yuc.wiki/) — 感谢長門有C多年来精心编制的新番季风表
- [Vue.js](https://vuejs.org/) & [DaisyUI](https://daisyui.com/) & [TailwindCSS](https://tailwindcss.com/)

---

## 📄 开源许可证

本项目基于 [MIT License](LICENSE) 开源发布。
欢迎提交 Issue 与 Pull Request！
