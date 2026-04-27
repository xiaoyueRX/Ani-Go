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
- **数据库路径**: `d:\pve\Ani-rss\ani-rss.db` (Windows) / `/data/ani-rss.db` (Docker)

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

- **语言**: Go 1.22+
- **数据库**: SQLite (via GORM + 纯 Go 驱动 `glebarez/sqlite`)
- **核心模式**: **接口驱动 (Interface-First)**
  - `Source`: 资源抓取
  - `Downloader`: 如下载器交互 (qB/TR/Aria2)
  - `Metadata`: 番剧元数据 (TMDB/BGM.tv)
  - `Organizer`: 文件整理规则
- **交互方式**:
  - Web UI + RESTful API
  - 插件系统 (HTTP Sidecar 模式)

---

## 4. 当前开发进度 (Progress)

- [x] **Phase 0: 项目初始化**
  - [x] 确定项目名称: `Ani-Go`
  - [x] 初始化 Go 模块与目录结构
  - [x] 定义核心接口 `internal/core/interfaces.go`
  - [x] 实现配置加载系统 (环境变量优先)
  - [x] 实现数据库初始化与 ORM 模型 (GORM)
- [x] **Phase 0.5: 代码上云**
  - [x] 成功修复 PowerShell 造成的编码破坏 (UTF-8)
  - [x] 建立 GitHub 仓库 `xiaoyueRX/Ani-rss` (仓库名保留为 Ani-rss)
  - [x] 完成首次代码 Push
- [ ] **Phase 1: 核心引擎实现** (NEXT)
  - [ ] Mikan RSS 解析器
  - [ ] qBittorrent 客户端集成
- [ ] **Phase 2: 整理与元数据**
- [ ] **Phase 3: Web UI 与 部署**

---

## 5. 待办事项与注意事项

- **Token 保护**: `MIKAN_RSS_URL` 绝对不能出现在代码库，必须通过环境变量注入。
- **编码规范**: 所有文件必须保持 UTF-8 (无 BOM) 格式。不要使用 PowerShell 的默认重定向 `>`。
- **同一系列识别**: 未来需重点攻克“同一系列不同库名”的归并逻辑（TMDB Collection 方案）。

---

*Last Updated: 2026-04-24*
