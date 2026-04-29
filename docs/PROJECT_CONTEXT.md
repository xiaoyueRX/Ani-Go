# Ani-Go 项目核心记忆 (Project Context)

> [!IMPORTANT]
> **此文件是项目的“灵魂核心”。** 如果你在新的环境或使用新的 AI 助手开始工作，请第一时间让它阅读此文件，以获取完整的项目背景和开发约束。

---

## 1. 开发者与环境快照 (Context)

### 极客背景
- **所有者**: xiaoyueRX
- **主力机**: AMD Ryzen 5 9600X (PBO 200) / 32GB DDR5 6000 C28
- **服务器**: PVE 9.1 (Systemd 绑定 MAC) -> fnOS (Docker 宿主)
- **网络拓扑**: 
  - 宿舍 (NAT4) 与家里 (IPv6+FRP) 物理隔离。
  - 唯一网关入口: Lucky v2.27.2 (HTTPS 端口: 16601/50929)。
  - **关键约束**: 严禁生成跨网段局域网广播，必须通过 Lucky 反代或内网穿透访问。

### 存储路径 (fnOS)
- **电视剧/番剧根目录**: `/vol2/1000/TV/Media/番剧`
- **剧场版根目录**: `/vol2/1000/TV/Media/剧场版`
- **数据库路径**: `d:\pve\Ani-Go\ani-go.db` (Windows) / `/data/ani-go.db` (Docker)

---

## 2. 项目起源与核心痛点

### 替代对象: ani-rss
- **痛点1**: 只能订阅 RSS，无法处理 RSS 覆盖不到的历史老番集数。
- **痛点2**: 数据库鲁棒性差，曾因 DNS 故障导致逻辑大规模崩溃。
- **痛点3**: 订阅管理手动程度高。

### Ani-Go 的终极目标
1. **Mikan 个人 RSS 全量同步**: 网页端一键订阅，后台自动发现并创建任务。
2. **历史全量补全 (Soul Feature)**: 能够爬取 Mikan 番剧详情页，自动抓取并下载全量历史种子。
3. **文件整理一体化**: 实现符合 Jellyfin/fnOS 刮削规范的自动重命名与目录层级创建。
4. **插件与大模型扩展**: 支持外挂插件和 LLM（OpenAI/Gemini/Ollama）进行剧集分类和系列识别。

---

## 3. 技术架构 (Stack)

- **语言**: Go 1.25+
- **数据库**: SQLite (via GORM + 纯 Go 驱动 `glebarez/sqlite`)
- **关键依赖**: `golang-jwt/jwt/v5`（JWT 鉴权）、`goquery`（HTML 解析）、`golang.org/x/crypto`（Bcrypt）
- **核心模式**: **接口驱动 (Interface-First)**
  - `Source`: 资源抓取
  - `Downloader`: 如下载器交互 (qB/TR/Aria2)
  - `Metadata`: 番剧元数据 (TMDB/BGM.tv)
  - `Organizer`: 文件整理规则
- **交互方式**:
  - Vue3 + DaisyUI Web UI + RESTful API（JWT 鉴权）
  - 插件系统 (HTTP Sidecar 模式)

---

## 4. 当前开发进度 (Progress)

- [x] **Phase 0: 项目初始化** ✅
  - [x] 确定项目名称: `Ani-Go`
  - [x] 初始化 Go 模块与目录结构
  - [x] 定义核心接口 `internal/core/interfaces.go`
  - [x] 实现配置加载系统 (环境变量优先)
  - [x] 实现数据库初始化与 ORM 模型 (GORM)
  - [x] 建立 GitHub 仓库并完成首次 Push
- [x] **Phase 1: 核心引擎 MVP** ✅
  - [x] Mikan RSS 解析器（8 种正则模式）
  - [x] qBittorrent 客户端集成
  - [x] 基础调度器（定时轮询）
  - [x] 基础文件整理
  - [x] EventBus 事件总线
- [x] **Phase 2: 历史补全与元数据** ✅
  - [x] Mikan HTML 全页爬取（历史补全）
  - [x] TMDB / BGM.tv 元数据集成
  - [x] GFW 镜像/代理自动回退
  - [x] 补全调度器
- [x] **Phase 3: Web UI 与部署** ✅
  - [x] Vue3 + DaisyUI 前端（登录、订阅管理、下载队列、设置）
  - [x] RESTful API + JWT 鉴权 + go:embed 前端嵌合
  - [x] Docker 多阶段构建 + CI/CD（GitHub Actions 多架构镜像）
- [x] **Phase 4: AI + 多下载器 + 插件** ✅
  - [x] AI 多协议辅助（OpenAI/Google/Anthropic/Ollama）
  - [x] qBittorrent / Transmission / Aria2 多下载器
  - [x] 插件系统（Webhook + Shell 脚本）
  - [x] 多资源站（Nyaa/ACG.RIP/AnimeTosho + MultiSource 聚合）
- [x] **Phase 5: 多平台消息通知** ✅
  - [x] 16 平台通知（Telegram/Discord/WeCom/Feishu/DingTalk/QQ/Slack/Matrix/LINE/WhatsApp/ServerChan/Bark/Pushover/Gotify/ntfy/Email）
  - [x] 自然语言任务解析器（正则 + AI 回退）
  - [x] EventBus 自动推送 + MultiNotifier 聚合广播
- [x] **Phase 6: 数据迁移** ✅
  - [x] AutoBangumi SQLite 数据导入工具
- **测试**: 108 个测试全通过

---

## 5. 待办事项与注意事项

- **Token 保护**: `MIKAN_RSS_URL` 绝对不能出现在代码库，必须通过环境变量注入。
- **编码规范**: 所有文件必须保持 UTF-8 (无 BOM) 格式。不要使用 PowerShell 的默认重定向 `>`。
- **同一系列识别**: 未来需重点攻克“同一系列不同库名”的归并逻辑（TMDB Collection 方案）。

---

*Last Updated: 2026-04-29*
