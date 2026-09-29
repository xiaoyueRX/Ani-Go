# CODEX.md - Ani-Go 全量工程上下文、架构蓝图与工作流转移指南

> **文件定位**：本文件是专门为 **OpenAI Codex / CodeX CLI / 智能体工作流** 打造的唯一全局上下文与架构圣经。它凝聚了从项目建立至今 190+ 轮深度交互、全部历史重构、源码级审查与缺陷修复、次世代微内核架构、MCP 协议体系以及未来规划的所有细节。
> **目标**：当你在任何终端使用 Codex 接管 Ani-Go 项目时，阅读本文档即可具备与主架构师完全一致的项目全景认知，实现 100% 零信息折损的自主开发与维护。

---

## 目录索引 (Table of Contents)
- [第一部分：核心身份、工程哲学与微内核宪法](#第一部分核心身份工程哲学与微内核宪法)
  - [1.1 系统定位与使命](#11-系统定位与使命)
  - [1.2 微内核解耦宪法（不可动摇的铁律）](#12-微内核解耦宪法不可动摇的铁律)
  - [1.3 核心技术栈与架构全景](#13-核心技术栈与架构全景)
  - [1.4 目录工程结构图谱](#14-目录工程结构图谱)
  - [1.5 数据模型与数据库设计](#15-数据模型与数据库设计)
  - [1.6 配置优先级三级定律与热生效机制](#16-配置优先级三级定律与热生效机制)
- [第二部分：全周期演进史、重大重构与全部缺陷清零实录](#第二部分全周期演进史重大重构与全部缺陷清零实录)
  - [2.1 项目演进时间线与关键里程碑](#21-项目演进时间线与关键里程碑)
  - [2.2 逐行审查挖出的重大缺陷与修复铁证（Zero-Defect 修复矩阵）](#22-逐行审查挖出的重大缺陷与修复铁证zero-defect-修复矩阵)
  - [2.3 安全防御基线与加固措施](#23-安全防御基线与加固措施)
- [第三部分：次世代核心子系统深度剖析](#第三部分次世代核心子系统深度剖析)
  - [3.1 全自动内部消歧引擎（三级漏斗与时间戳对齐算法）](#31-全自动内部消歧引擎三级漏斗与时间戳对齐算法)
  - [3.2 原生 MCP Server 实现规范与 AI Agent 治理](#32-原生-mcp-server-实现规范与-ai-agent-治理)
  - [3.3 插件生态系统与物理断电隔离机制](#33-插件生态系统与物理断电隔离机制)
  - [3.4 多下载器引擎（qB / Transmission / Aria2）动态切换](#34-多下载器引擎qb--transmission--aria2动态切换)
  - [3.5 媒体整理引擎与规范化命名](#35-媒体整理引擎与规范化命名)
- [第四部分：双环境开发规范、CI/CD 流水线与部署运维](#第四部分双环境开发规范cicd-流水线与部署运维)
  - [4.1 双平台开发与编译拓扑（Windows 11 + WSL2）](#41-双平台开发与编译拓扑windows-11--wsl2)
  - [4.2 依赖管理红线（Go 1.25+ / Node 24 / 纯 Go 禁令）](#42-依赖管理红线go-125--node-24--纯-go-禁令)
  - [4.3 GitHub Actions CI/CD 流水线设计与 GHCR 镜像](#43-github-actions-cicd-流水线设计与-ghcr-镜像)
  - [4.4 生产部署实战方案（Docker / fnOS / PVE / 二进制）](#44-生产部署实战方案docker--fnos--pve--二进制)
- [第五部分：未来蓝图待办、未尽议题与 Codex 自主开发军规](#第五部分未来蓝图待办未尽议题与-codex-自主开发军规)
  - [5.1 待办事项与次世代功能储备 (Roadmap)](#51-待办事项与次世代功能储备-roadmap)
  - [5.2 Codex 智能体自主开发行动铁律](#52-codex-智能体自主开发行动铁律)
  - [5.3 核心 CLI 运维与测试速查命令表](#53-核心-cli-运维与测试速查命令表)

---

# 第一部分：核心身份、工程哲学与微内核宪法

## 1.1 系统定位与使命
**Ani-Go** 是一款面向个人与家庭 NAS 用户打造的**极轻量、高性能、工业级自动化追番、下载调度、媒体整理与元数据协同系统**。
- **核心追求**：
  - **极致轻量**：纯 Go 单二进制运行，常驻内存仅 **15MB ~ 30MB**，CPU 待机占用几乎为 0%，老旧瘦客户机、NAS、路由器/软路由均可流畅无阻运行。
  - **安静坚韧**：作为一个后台“静默苦力”，用户填好订阅后，系统全天候自动轮询 RSS、消歧解析标题、调度下载器、硬链接整理文件并通知 Jellyfin/Emby/Plex，无需任何人工日常干预。
  - **单文件嵌合**：前端 Vue 3 SPA 打包生成静态文件后，通过 Go 1.16+ `//go:embed` 技术无缝内嵌进后端可执行文件，实现零外部运行时依赖的单文件绿色部署。

## 1.2 微内核解耦宪法（不可动摇的铁律）
在项目演进中，系统曾一度因堆叠功能（如多渠道推送、死信队列、日志看板、AI 模型等）而出现体积与复杂度膨胀。为此，团队确立了不可动摇的《微内核宪法》（收录于 `docs/00_DESIGN_PHILOSOPHY.md`），任何代码开发均必须遵守以下二选一裁决：

```
                ┌──────────────────────────────────────┐
                │          Ani-Go 微内核 (Core)         │
                │ ──────────────────────────────────── │
                │ 1. RSS 订阅抓取 (Mikan / Nyaa 等)     │
                │ 2. 基础标题消歧与集数提取              │
                │ 3. 跨下载器调度 (qB/TR/Aria2)         │
                │ 4. 规范文件整理与零占用硬链接          │
                │ 5. 媒体服务器通知 (Jellyfin/Emby)     │
                └──────────────────┬───────────────────┘
                                   │ 事件总线 / 钩子
                                   ▼
                ┌──────────────────────────────────────┐
                │         可插拔插件生态 (Plugins)       │
                │ ──────────────────────────────────── │
                │ • 16 渠道聚合通知 (DingTalk/Bark 等) │
                │ • 本地 NFO / TMDB 刮削与海报落盘     │
                │ • 种子生命周期回收与死种智能换源       │
                │ • 极客重命名 / 追番历史导入导出      │
                │ • 原生 MCP Server 服务扩展            │
                └──────────────────────────────────────┘
```

1. **核心主干（Core）唯一定义**：
   - 严格仅限五步闭环流水线：**「RSS抓取 ──► 标题消歧 ──► 下载调度 ──► 规范硬链接 ──► 刷新Jellyfin」**。
   - 这五步直接内置于微内核中，保持纯粹、极简、无依赖。
2. **周边功能（Plugins）唯一定义**：
   - 除上述五步外的**所有**功能（包括消息推送、AI大模型消歧、NFO刮削、备份迁移、统计图表等），必须且只能作为独立插件（Plugin）存在。
   - **物理断电铁律（Power-cut Isolation）**：插件被禁用时，必须完整销毁所有后台协程、网络监听与轮询计时器，实现 **0 协程、0 网络、0 冗余日志、0 内存膨胀**。绝不允许出现“插件已禁用但后台还在报错重试”的缺陷。

## 1.3 核心技术栈与架构全景
- **后端内核**：Go 1.25+（完全遵循标准库，无臃肿三方框架，HTTP 服务基于原生 `http.ServeMux` 增强路由）。
- **数据库驱动**：`github.com/glebarez/sqlite`（纯 Go 实现的 SQLite 驱动，**绝对禁止**引入依赖 gcc 的 CGO 驱动如 `mattn/go-sqlite3`，确保 `CGO_ENABLED=0` 跨架构原生交叉编译）。
- **ORM 框架**：GORM v1.31+，开启预编译语句缓存（`PrepareStmt: true`），WAL 模式与并发读写锁优化。
- **前端架构**：Vue 3.5 + TypeScript + Vite + TailwindCSS + Lucide Icons，支持响应式多端自适应布局（桌面/平板/手机）。
- **协议支撑**：Native Model Context Protocol (MCP) Server（基于 Server-Sent Events SSE + Bearer 鉴权）。
- **部署架构**：Docker Multi-arch（`linux/amd64`, `linux/arm64`）自动推送 GHCR，支持二进制裸机 Systemd 服务运行。

## 1.4 目录工程结构图谱
```
Ani-Go/
├── cmd/                        # 扩展命令行工具集
├── internal/                   # 后端核心业务内聚包（外部不可直接引用）
│   ├── ai/                     # AI 大模型客户端接口（DeepSeek/OpenAI/Gemini/Ollama）
│   ├── api/                    # RESTful API 路由定义、Handlers 与 WebSocket 控制器
│   ├── auth/                   # JWT 鉴权、Bcrypt 密码加密与免密策略中间件
│   ├── backup/                 # 数据备份与一键恢复引擎
│   ├── config/                 # 环境变量与数据库配置加载器（三级优先律）
│   ├── core/                   # 核心抽象接口（Source, Downloader, Organizer, EventBus 等）
│   ├── database/               # SQLite GORM 实体定义与数据库初始化迁移
│   ├── downloader/             # 下载器抽象层（qBittorrent / Transmission / Aria2 / 动态路由）
│   ├── event/                  # 内存高性能发布-订阅事件总线（EventBus）
│   ├── httpx/                  # 统一防卡死 HTTP Client（连接池复用、超时保护）
│   ├── mcp/                    # 原生 Model Context Protocol 服务端实现
│   ├── metadata/               # 番剧元数据客户端（Bangumi.tv / TMDB / 官方日历映射）
│   ├── migrate/                # 历史版本数据迁移器
│   ├── notifier/               # 聚合推送核心（v2 多渠道分发、重试机制、通道管理器）
│   ├── organizer/              # 文件规范化整理（硬链接/移动、层级建立、路径防越界）
│   ├── parser/                 # 种子标题消歧与解析（正则漏斗、单集时间戳对齐算法）
│   ├── plugin/                 # 微内核插件生命周期管理器与内置插件集
│   ├── scheduler/              # 定时任务轮询引擎（Cron 表达式、任务编排）
│   ├── search/                 # 多资源站聚合搜索与搜索词扩展器
│   └── source/                 # 资源站采集驱动（Mikan / Nyaa / ACG.RIP / AnimeTosho）
├── web/                        # 前端 Vue 3 全栈工程
│   ├── src/
│   │   ├── api/                # 前端 Axios 请求封装与统一错误拦截
│   │   ├── components/         # 通用 UI 组件库
│   │   ├── composables/        # 组合式函数（useVersion, useToast, useWebSocket 等）
│   │   ├── views/              # 业务页面（订阅、时间表、搜索、下载、设置、插件、日志）
│   │   └── router/             # 路由守卫与动态跳转
│   ├── package.json            # 前端依赖配置（Node 24 / Vite）
│   └── dist/                   # 编译后产物（通过 embed.go 内嵌）
├── docs/                       # 标准工业级工程文档知识库
│   ├── 00_DESIGN_PHILOSOPHY.md # 设计哲学与微内核宪法
│   ├── 01_ARCHITECTURE.md      # 核心系统架构白皮书
│   ├── 02_PLUGIN_SPEC.md       # 插件开发规范与接口白皮书
│   ├── 03_MCP_SPEC.md          # MCP 协议服务端与工具集规范
│   ├── 04_AUTOPARSER_SPEC.md   # 全自动三级消歧引擎规范
│   └── 05_DEPLOYMENT.md        # 生产部署与容器化实践指南
├── Dockerfile                  # 多阶段极速交叉编译 Docker 构建配置
├── docker-compose.yml          # 标准 Docker Compose 生产运行模板
├── install.sh                  # Linux / macOS 一键安装与服务编排脚本
├── embed.go                    # Go 静态资源内嵌胶水代码
├── main.go                     # 应用程序唯一启动入口与生命周期总装
├── go.mod / go.sum             # Go 模块依赖树（Go 1.25+）
├── README.md / README_EN.md    # 中英文官方说明文档
├── CHANGELOG.md                # 标准版本变更履历
└── CODEX.md                    # 本文件（Codex 工作流转移全局圣经）
```

## 1.5 数据模型与数据库设计
系统基于 SQLite 运行，采用单连接写、多连接并发读的 WAL 模式，核心实体如下：
- `Subscription`（订阅表）：存储番剧元数据、Mikan/Bangumi ID、季度、期望质量、过滤规则、更新状态。
- `Torrent`（种子表）：记录抓取到的种子 Hash、标题、链接、发布时间、关联订阅 ID、下载状态、消歧结果。
- `Setting`（全局键值对设置表）：采用 Key-Value 结构持久化所有 Web 界面可修改的配置项（优先于默认值）。
- `PluginConfig`（插件配置表）：记录各个插件的启用/禁用状态及其专属 JSON 配置块。
- `User`（用户表）：存储管理员账号与 Bcrypt 加密哈希（支持内网免密配置）。
- `NotificationRecord`（通知投递历史表）：记录投递事件、推送渠道、成功/失败状态与错误信息。

## 1.6 配置优先级三级定律与热生效机制
Ani-Go 严格执行以下**三级配置决议律**：
```
Level 1: 操作系统环境变量 (OS Environment Variables)  [最高优先级，不可被覆盖]
   │
   ▼
Level 2: 数据库持久化配置 (SQLite `settings` 表)     [中优先级，Web 设置实时写入]
   │
   ▼
Level 3: 程序代码内置默认值 (In-memory Defaults)     [最低兜底优先级]
```
- **热生效原理**：`internal/config` 提供了 `MergeFromSettings` 接口。当用户在 Web 设置页点击保存时，配置立刻更新至数据库，并调用全局热生效机制通知各模块（例如：修改下载器连接参数时动态重建客户端连接池；修改插件开关时触发物理启停）。

---

# 第二部分：全周期演进史、重大重构与全部缺陷清零实录

## 2.1 项目演进时间线与关键里程碑
1. **初期奠基阶段 (v0.1.0 - v0.3.0)**：
   - 实现了基于 Mikan RSS 的基础订阅与抓取流程。
   - 引入 qBittorrent 基础 API 交互，建立正则分词器。
   - 实现了初始的 Vue 3 单页面与前端内嵌机制。
2. **多源与架构成型阶段 (v0.4.0 - v0.5.0)**：
   - 扩展支持了 Nyaa、ACG.RIP、AnimeTosho 资源站聚合。
   - 引入了 Aria2 与 Transmission 下载器支持，实现了多下载器接口抽象。
   - 搭建了 16 渠道通知聚合系统（Telegram, Bark, Server酱, 飞书, 钉钉等）。
3. **架构大反思与微内核重塑 (v0.5.2 - v0.5.4)**：
   - 痛感周边功能臃肿挤占核心，正式推行微内核与插件化改革。
   - 提出 MCP 原生支持，将外部 AI 定位为“手和脚”的操控方。
   - 研发单集放送时间对齐算法，解决奇葩字幕组命名歧义。
4. **工业级整肃与缺陷清零 (v0.6.0 Next-Gen)**：
   - 全面完成 20 个核心包逐行审计，清零全部并发竞态与隐藏 Bug。
   - 统一对齐 Go 1.25+ 与 Docker 多架构编译流水线。
   - 发布 GitHub Release v0.6.0，提供 7 大架构预编译包，全面构建 GHCR 容器镜像。

## 2.2 逐行审查挖出的重大缺陷与修复铁证（Zero-Defect 修复矩阵）

### 缺陷 1：[CRITICAL] 数据库配置合并失效与默认值锁死
- **位置**：[`internal/config/config.go`](file:///d:/project/Ani-Go/internal/config/config.go)
- **根因**：原 `MergeFromSettings` 函数中存在判断逻辑 `if c.Field == ""`。但系统启动时 `defaults()` 已经为多数配置赋予了默认字串（如 `QB_HOST = "http://localhost:8081"`）。导致用户在 Web 端无论如何修改设置并保存，系统重启后均因字段非空而被默认值强制锁死，配置完全无法生效。此外，还存在 `DEFAULT_DOWNLOADER` 与 `DOWNLOADER_DEFAULT` 键名不兼容、遗漏合并字段数十个的缺陷。
- **修复**：重构 `MergeFromSettings`，严格以 `os.Getenv(k) == ""` 作为第一准则——只要环境变量未设置，数据库持久化值必须无条件覆写结构体字段；并实现全字段合并映射与双别名兼容。

### 缺陷 2：[HIGH] qBittorrent 并发 Data Race 与登录风暴
- **位置**：[`internal/downloader/qbittorrent.go`](file:///d:/project/Ani-Go/internal/downloader/qbittorrent.go)
- **根因**：
  1. 重试逻辑中直接并发写 `q.httpClient.Jar`，触发 Go 运行时并发数据竞争（Data Race）；
  2. `q.login(ctx)` 未加互斥锁，多协程并发调用时触发登录风暴，疯狂请求 qB WebUI；
  3. `List()` 轮询前每次都冗余执行全量登录检查，造成双倍请求风暴。
- **修复**：移除修改 `Jar` 的危险逻辑；使用 `loginMu` 互斥锁保护登录过程；引入 30 分钟登录态缓存与 401 自动惰性重登机制；在 `parseQBittorrentList` 遇到空 Body 时容错 `io.EOF`，测试通过 `go test -race` 严密验证。

### 缺陷 3：[HIGH] Aria2 任务删除异常与磁盘媒体文件泄漏
- **位置**：[`internal/downloader/aria2.go`](file:///d:/project/Ani-Go/internal/downloader/aria2.go)
- **根因**：Aria2 RPC 接口中，正在下载的任务需调用 `remove`，已完成/停止的任务需调用 `removeDownloadResult`。原代码一刀切直接调用后者，导致下载中任务报错；且 Aria2 RPC 协议原生绝不删除本地磁盘文件，导致用户勾选“同时删除文件”时本地磁盘文件永久泄漏。
- **修复**：在执行 RPC 删除前，通过 `aria2.tellStatus` 预提取关联的本地文件物理路径；随后执行级联降级删除（`remove` -> 失败降级至 `removeDownloadResult` -> 再失败降级至 `forceRemove`）；若 `deleteFiles=true`，则由 Ani-Go 主动安全清理磁盘文件及 `.aria2` 控制文件。

### 缺陷 4：[MEDIUM] TMDB 纯数字 ID 导致 404
- **位置**：[`internal/metadata/tmdb.go`](file:///d:/project/Ani-Go/internal/metadata/tmdb.go)
- **根因**：`GetExternalIDs` 拼接路径假定 ID 为 `tv/123`。若传入纯数字 ID（如 `"12345"`），URL 被拼接为 `//12345/external_ids`，直接导致 TMDB API 404 挂死。
- **修复**：对传入 ID 实施 `strings.Trim(id, "/")` 标准化清洗，缺省前缀时自动补齐 `tv/` 前缀。

### 缺陷 5：[MEDIUM] 整理器路径逃逸防越界（Path Traversal Boundary Defense）
- **位置**：[`internal/organizer/organizer.go`](file:///d:/project/Ani-Go/internal/organizer/organizer.go)
- **根因**：在番剧名称或文件名包含 `../` 或不规则相对路径时，硬链接/移动操作可能穿透基础目录，向系统敏感目录写入文件。
- **修复**：在执行任何文件系统物理操作前，严格使用 `filepath.Clean`、`filepath.Rel` 与前缀校验，强行约束目标路径必须属于 `TV_BASE_PATH` 或 `ANIME_DIR` 子目录，防御任何形式的目录穿透攻击。

### 缺陷 6：[CRITICAL] Dockerfile 与 Go 1.25 依赖冲突
- **位置**：[`Dockerfile`](file:///d:/project/Ani-Go/Dockerfile), [`go.mod`](file:///d:/project/Ani-Go/go.mod)
- **根因**：`go.mod` 及关键加密库（`golang.org/x/crypto v0.49.0` 等）严格要求 `go >= 1.25.0`。此前曾误将基础镜像降级为 `golang:1.24-alpine`，在启用 `GOTOOLCHAIN=local` 的容器构建环境下触发构建中断退出。
- **修复**：将 Dockerfile 构建阶段彻底升级至官方 `golang:1.25-alpine`，结合 `CGO_ENABLED=0` 原生交叉编译，全线通过 `linux/amd64` 与 `linux/arm64` 双架构 GitHub Actions 自动化编译与推送。

## 2.3 安全防御基线与加固措施
1. **内网免密体验与安全平衡**：
   - 考虑家庭内网使用场景，支持在控制台开启“内网免密访问”选项。
   - 针对公网暴露环境，保留 Bcrypt 强哈希（Cost=12）密码校验；每次服务冷重启时由 `crypto/rand` 随机生成 32 字节 JWT 密钥，防止历史 Token 泄露。
   - 实施登录防暴力破解限制（连续失败 5 次自动锁定 IP 10 分钟）。
2. **反向代理头标标准化**：
   - 接入 `ProxyHeadersMiddleware`，深度兼容 Lucky、Nginx、Caddy、Traefik 等反代服务传递的 `X-Forwarded-For`、`X-Forwarded-Proto` 与 `X-Real-IP`，保证客户端真实 IP 识别准确。

---

# 第三部分：次世代核心子系统深度剖析

## 3.1 全自动内部消歧引擎（三级漏斗与时间戳对齐算法）
在全自动追番流程中，最大的痛点是国内各字幕组发布的种子标题格式千奇百怪（例如：全剧合集标注为 `[25]`、季号缺失、正片与特别篇 SP 混淆）。Ani-Go 首创了**三级消歧漏斗架构**：

```
                      原始种子标题 (RSS Title)
                                │
                                ▼
         ┌──────────────────────────────────────────────┐
         │ 第一级：本地正则分词清洗 (0ms / 0 Token)       │
         │ - 提取分辨率 (1080p/4K)、格式 (MP4/MKV)、字幕组 │
         │ - 识别标准集数格式 (E01, [01], EP01, #01 等)   │
         └──────────────────────┬───────────────────────┘
                                │ 未决歧义 / 模糊集数
                                ▼
         ┌──────────────────────────────────────────────┐
         │ 第二级：Bangumi 官方放送时间对齐 (0 Token)   │
         │ - 读取 Bangumi 官方每日单集放送表 (airdate)    │
         │ - 比对种子发布时间 (pubDate) 与容差窗口 (0~72h) │
         │ - 100% 确定性解析模糊季号与大集数 (解决 95% 歧义) │
         └──────────────────────┬───────────────────────┘
                                │ 极端冷门 / 奇葩异常命名
                                ▼
         ┌──────────────────────────────────────────────┐
         │ 第三级：大模型带上下文智能消歧 (受控 Token)     │
         │ - 携带番剧总集数、当前季度、上下文元数据发起推断 │
         │ - 终身唯一 SQLite 缓存 (相同标题绝不二次调用)    │
         │ - 每日配额防爆上限 (防止历史 RSS 产生意外账单)   │
         └──────────────────────────────────────────────┘
```

1. **第一级（本地正则，0 成本）**：
   - 采用预编译的高性能正则表达式集，提取方括号内的发布组信息、清晰度、压制格式及标准集数。
2. **第二级（单集放送时间戳对齐算法，0 Token 成本）**：
   - 核心代码：[`internal/parser/airdate_matcher.go`](file:///d:/project/Ani-Go/internal/parser/airdate_matcher.go)。
   - **算法机理**：当遇到如 `[25]` 这种既可能是“第一季第25集”也可能是“第二季第1集”的标题时，引擎调取 Bangumi 官方该番剧的分集首播时间表。如果某个种子发布在 2026 年 10 月 5 日，而第二季第 1 集官方开播日恰为 2026 年 10 月 5 日，在设定容差窗口（如 0~72 小时）内命中，则自动且精准地将该种子校准为 `S02E01`。该算法完全免费、无需消耗任何大模型 Token，解决了 95% 的中文字幕组命名歧义。
3. **第三级（带上下文的 AI 智能推断，强防爆）**：
   - 当一、二级无法得出高置信度结果时，由系统调用配置的 AI 模型（支持 DeepSeek、OpenAI、Gemini 或本地 Ollama）。
   - **Token 防爆盾（Blowout Defense）**：
     - **Web 界面高能警示**：在设置页清晰展示黄色/红色警示横幅，提示用户该功能涉及第三方 API 费用；
     - **终身唯一缓存**：以 `SHA256(RawTitle)` 作为主键在 SQLite 中持久化结果，同一标题终身仅调用一次；
     - **每日调用阈值**：设置每日最大调用次数（如 50 次），达到阈值后自动挂起，绝不产生账单失控。

## 3.2 原生 MCP Server 实现规范与 AI Agent 治理
Ani-Go 坚决不走“在后端塞进庞大模型 SDK 自己做聊天对话框”的伪 AI 弯路，而是践行 UNIX 哲学——**Ani-Go 只做手和脚，外部智能体（Claude Desktop, Cursor, 私有 Agent）才是大脑**。
- **协议端点**：`/api/mcp/sse`（支持标准的 Server-Sent Events 流式传输与 JSON-RPC 2.0 规范，附带 Bearer Token 安全认证）。
- **工具集设计（Tools）**：
  1. **业务操作类**：
     - `search_anime(keyword)`：在多资源站聚合搜索番剧及种子；
     - `add_subscription(mikan_id, title, ...)`：一键添加自动追番订阅；
     - `delete_subscription(id)`：安全删除订阅或移入回收站；
     - `list_subscriptions(page, status)`：查询当前订阅状态与更新进度；
     - `get_downloads()`：实时获取底层 qB / Aria2 下载队列与速度。
  2. **系统治理类（AI 运维管家）**：
     - `list_plugins()`：列出全系统已安装插件及其运行态；
     - `toggle_plugin(plugin_id, enabled)`：一句话开启或关闭某个插件（如“帮我关掉推送通知”）；
     - `update_plugin_config(plugin_id, config)`：调整插件私有参数（如“帮我把 Bark Key 换成 xxx”）；
     - `get_settings()` / `update_settings(key, value)`：查询或调整全局系统配置。
  3. **动态工具扩展（Dynamic Mounting）**：
     - 任何插件若实现了 `MCPExtension` 接口，其私有工具将在插件启用时**自动注册**到 MCP 工具列表；在插件关闭时**即刻注销**。外部 AI 永远无法调用已停用插件的工具。

## 3.3 插件生态系统与物理断电隔离机制
所有非核心流水线功能统一收归插件生命周期管理器（`internal/plugin/manager.go`）：
- **标准插件接口**：
  ```go
  type Plugin interface {
      ID() string
      Name() string
      Version() string
      Description() string
      Init(ctx context.Context, bus event.Bus, db *gorm.DB) error
      Start(ctx context.Context) error
      Stop(ctx context.Context) error
      ConfigSchema() []ConfigItem
  }
  ```
- **物理断电铁律（Power-cut Isolation）**：
  - 调用 `Stop(ctx)` 时，必须主动取消传递给后台 Worker 的上下文（`cancel()`）；
  - 必须断开所有的 HTTP 长轮询与推送重试协程；
  - 必须将内部状态置为 Inactive，停止监听全局 EventBus 事件；
  - 界面设置项与数据统计联动：当插件被关闭时，Web 页面自动灰显或隐藏关联设置卡片，避免“功能已关但界面还在报错”的视觉污染。
- **核心内置插件阵容**：
  - `notifier`：16 渠道消息推送聚合（支持 Bark, Server酱, 飞书, 钉钉, Telegram, 企业微信, PushPlus, Gotify, Discord, Webhook 等）；
  - `yuc_schedule`：长门番堂（yuc.wiki）新番时间表抓取与周历展示；
  - `geek_naming`：极客文件命名注入与元数据自定义；
  - `data_migrate`：从 AutoBangumi 或其他工具导入历史追番与规则。

## 3.4 多下载器引擎（qB / Transmission / Aria2）动态切换
- **抽象接口**：`internal/core/interfaces.go` 中的 `Downloader` 接口抽象了所有下载操作。
- **支持实现**：
  - **qBittorrent**：基于 Web API v2，实现了会话复用、登录锁保护、30 分钟缓存与 401 自动惰性重登；
  - **Transmission**：基于 RPC 协议，自动处理 `X-Transmission-Session-Id` 校验失败的 409 握手与会话续期；
  - **Aria2**：基于 JSON-RPC 协议，支持 Secret 认证，实现了 `remove` 与 `removeDownloadResult` 的级联降级和本地残留文件级联清理。
- **动态调度器 (`DynamicDownloader`)**：
  - 在微内核中充当透明代理门面；用户在 Web 控制台热切换默认下载器时，无需重启系统，下一条下载任务即刻通过新下载器派发。

## 3.5 媒体整理引擎与规范化命名
- **物理处理模式**：
  - **硬链接模式（`useHardLink = true`，推荐）**：源文件保留在下载目录维持做种（PT 友好），媒体库目录建立硬链接，秒级完成且 0 额外磁盘开销。跨文件系统或物理分区不支持时，平滑降级为“复制”。
  - **移动模式（`useHardLink = false`）**：下载完成后直接移入媒体库，发生跨盘错误时触发“分块流式拷贝 + 哈希校验 + 销毁源文件”。
- **标准 Jellyfin / Emby 目录规范**：
  ```
  {TV_BASE_PATH}/
  └── {AnimeTitle}/
      └── Season {SeasonNumber}/
          └── {AnimeTitle} - S{SS}E{EE} - {Resolution} [{Group}].{Ext}
  ```
- **实时刮削触发**：整理完成后，通过 Webhook/HTTP 请求自动通知 Jellyfin/Emby 刷新指定剧集媒体库，无需等待媒体服务器整库定时扫描。

---

# 第四部分：双环境开发规范、CI/CD 流水线与部署运维

## 4.1 双平台开发与编译拓扑（Windows 11 + WSL2）
- **拓扑架构**：
  - **宿主开发机**：Windows 11 主机，物理工程目录位于 `D:\project\Ani-Go`。
  - **WSL2 子系统**：Ubuntu 运行环境，挂载路径为 `/mnt/d/project/Ani-Go`，安装了原生 Go 1.25.0 与 Node.js 24。
  - **家庭云节点**：PVE 宿主机（外网反代 `xn--kkrv96l5zbusc.icu:50924`）与内网飞牛OS（fnOS 10.10.10.2），通过 WireGuard / Tailscale 组网。
- **最佳协作准则**：
  - **代码与 Git 提交**：直接在 Windows 宿主机 IDE（或 WSL 中）执行，两者共享同一 NTFS 底层存储；
  - **前端构建**：在 Windows 宿主机通过 `npm run build` 打包 `web/dist`；
  - **后端测试与跨编译**：在 WSL2 中执行 `go test -race ./...` 与交叉编译，确保 Linux 下的高性能与并发安全。

## 4.2 依赖管理红线（Go 1.25+ / Node 24 / 纯 Go 禁令）
1. **Go 1.25+ 绝对红线**：
   - `go.mod` 声明为 `go 1.25.0`，底层间接库（如 `golang.org/x/crypto v0.49.0`）严格依赖 Go 1.25。构建镜像与本地环境必须保持 Go 1.25+。
2. **纯 Go 禁令 (CGO_ENABLED=0)**：
   - 严禁引入任何需要 C 编译器（gcc/clang）的第三方依赖；
   - 数据库必须使用 `github.com/glebarez/sqlite`。保证在任何平台无需安装 C 工具链即可瞬时编译。
3. **前端构建标准**：
   - Node.js 必须保持 24+，npm 依赖锁定在 `web/package-lock.json`。

## 4.3 GitHub Actions CI/CD 流水线设计与 GHCR 镜像
代码仓库配有三大自动化 GitHub Actions 驱动流程（全部稳定运行于 GitHub 官方 Runner）：

1. **持续集成 (`.github/workflows/ci.yml`)**：
   - 触发时机：`push` 到 `main` 分支、发起 `pull_request`。
   - 运行步骤：Node 24 构建前端 `web/` ➔ Go stable 运行 `go test -race ./...` 单元与并发竞态测试。
2. **自动化发布 (`.github/workflows/release.yml`)**：
   - 触发时机：推送 `v*` 语义化标签（如 `v0.6.0`）。
   - 运行步骤：构建前端生产包 ➔ 启动纯 Go 交叉编译矩阵 ➔ 打包 7 大平台二进制包（Linux amd64/arm64/armv7, Darwin amd64/arm64, Windows amd64/arm64）➔ 生成 `checksums.txt` SHA256 校验和 ➔ 发布至 GitHub Releases。
3. **Docker 多架构自动编译与推送 (`.github/workflows/docker-build.yml`)**：
   - 触发时机：`main` 分支推送、`v*` 标签推送。
   - 编译机制：多阶段构建，Stage 1 使用 Node 24 编译前端，Stage 2 使用 `golang:1.25-alpine` 原生交叉编译，Stage 3 组装 Alpine 3.21 极简镜像。
   - 产物推送：自动打标推送至 GitHub Container Registry (GHCR)：
     - `ghcr.io/xiaoyuerx/ani-go:latest`
     - `ghcr.io/xiaoyuerx/ani-go:0.6.0`
     - `ghcr.io/xiaoyuerx/ani-go:0.6`
     - `ghcr.io/xiaoyuerx/ani-go:sha-<commit>`

## 4.4 生产部署实战方案（Docker / fnOS / PVE / 二进制）

### 方案 A：Docker Compose 极简运行（推荐）
```yaml
services:
  anigo:
    image: ghcr.io/xiaoyuerx/ani-go:latest
    container_name: anigo
    restart: unless-stopped
    ports:
      - "20001:20001"
    environment:
      - TZ=Asia/Shanghai
      - PUID=1000
      - PGID=1000
      - PORT=20001
    volumes:
      - ./data/db:/data/db
      - /mnt/media:/data/media
      - /mnt/downloads:/data/downloads
```

### 方案 B：Linux 一键脚本安装
```bash
curl -fsSL https://raw.githubusercontent.com/xiaoyueRX/Ani-Go/main/install.sh | bash
```

### 方案 C：二进制裸机运行
从 [GitHub Releases](https://github.com/xiaoyueRX/Ani-Go/releases/latest) 下载对应系统架构的归档包，解压后单文件直接启动：
```bash
tar -zxvf ani-go-v0.6.0-linux-amd64.tar.gz
cd ani-go-v0.6.0-linux-amd64
./anigo
```

---

# 第五部分：未来蓝图待办、未尽议题与 Codex 自主开发军规

## 5.1 待办事项与次世代功能储备 (Roadmap)
在之前的深度架构推演中，团队规划了若干高度契合微内核理念的扩展插件，供后续由 Codex 主导实现：

### 1. 种子生命周期回收与死种自愈 (`dead_torrent_healer`)
- **痛点**：追冷门老番时，添加种子后常年 0 Peers（0.0% 卡死）；热播番下载完毕后长期占用做种队列与 NAS 存储空间。
- **规划方案**：
  - **死种探测**：设定死种超时阈值（如超过 72 小时进度为 0% 且 Peer 为 0），触发 `torrent.dead` 事件；
  - **自动换源**：调用搜索聚合引擎（Nyaa / AnimeTosho / ACG.RIP），自动根据标题、季数、集数重搜同类种子，无感替换并通知用户；
  - **做种回收**：支持按分享率阈值（如 Ratio >= 2.0）或做种时长（如 > 14 天）自动在下载器中安全清理已做种完毕的任务。

### 2. 本地 NFO 与精美海报就地刮削 (`local_nfo_scraper`)
- **痛点**：Jellyfin 在无公网外联或刮削网络不稳定时，无法显示剧集简介与海报；而 Ani-Go 本身已从 Bangumi 获取了 100% 准确的元数据。
- **规划方案**：
  - 监听 `file.organized` 事件；
  - 将番剧元数据格式化写入媒体目录的标准 `tvshow.nfo` 和 `S01E01.nfo`；
  - 同步拉取高分辨率海报并命名为 `poster.jpg` 与 `season01-poster.jpg`，让 Jellyfin / Emby 100% 秒读本地元数据。

### 3. 整理历史映射与一键撤销反悔机制 (`undo_organizer`)
- **痛点**：如果用户误配了命名规则或因特定字幕组识别偏差导致大量文件重命名/移位，用户想恢复原始状态极为困难。
- **规划方案**：
  - 在 SQLite 中建立 `organize_records` 履历表（记录 `SourcePath`, `TargetPath`, `ActionType`, `CreatedAt`）；
  - 提供 REST API 与 Web 交互入口，支持“整理前预览”与“一键回退到整理前状态”。

## 5.2 Codex 智能体自主开发行动铁律
当 Codex 作为主工程师接管并推进 Ani-Go 代码库时，**必须严格恪守以下行为军规**：

1. **绝对禁止中二与夸张言论 (Professional & Disciplined)**：
   - 交流、代码注释、Git Commit 提交信息及对外文档中，严禁出现任何中二、浮夸、轻浮或夸大其词的表述；
   - 保持严谨、专业、克制的大厂工业级工程师语态。
2. **微内核解耦红线 (Strict Decoupling)**：
   - 永远不得将非核心流水线逻辑（包括通知、刮削、图表、迁移等）直接硬编码写入 `internal/core`、`main.go` 或基础整理模块；
   - 任何新特性开发前必须自问：“这是否属于核心五步？”如果不是，强制以独立插件形式挂载至 `internal/plugin`。
3. **CGO 绝对禁止令 (Pure Go Guarantee)**：
   - 永远不得引入依赖 CGO 的三方库（如 `mattn/go-sqlite3`），确保 `CGO_ENABLED=0` 能够在所有目标平台上保持单文件极速交叉编译。
4. **非阻塞与资源零泄露 (Resource Safety)**：
   - 任何后台协程必须绑定可取消的 `context.Context`；
   - 网络客户端必须复用 `internal/httpx` 并声明明确超时，杜绝协程泄漏；
   - 插件停用时必须实现物理断电，确保 0 协程、0 网络、0 冗余日志。
5. **双端构建与测试闭环 (Verification Mandate)**：
   - 涉及后端改动后，必须执行 `go test -race ./...` 确保单元与竞态测试 100% 通过；
   - 涉及前端改动后，必须在 `web/` 执行 `npm run build`，确保编译无报错且产物内嵌正常；
   - 每次发布新版本必须同步推进 `CHANGELOG.md` 与前端 `useVersion.ts`。

## 5.3 核心 CLI 运维与测试速查命令表

### 1. 后端构建与测试 (在 WSL2 或 Linux 终端)
```bash
# 运行全部单元测试及并发竞态检测 (强制无 Race Condition)
go test -race -v ./...

# 编译当前平台可执行二进制
go build -ldflags="-s -w" -o anigo .

# 极速交叉编译 Linux ARM64 (纯 Go 原生极速构建，无需 QEMU)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -trimpath -o anigo-arm64 .
```

### 2. 前端构建与测试 (在 Windows PowerShell 或 Linux 终端)
```bash
# 进入前端工程目录
cd web

# 安装依赖
npm ci

# 启动本地热重载开发服务器
npm run dev

# 生产环境打包 (输出至 web/dist)
npm run build
```

### 3. 本地全栈联调与部署 (Windows 终端)
```bash
# 编译并启动服务 (默认端口 20001)
./ani-go.exe

# 浏览器访问
http://localhost:20001
```

### 4. Git 规范提交与发版工作流
```bash
# 暂存与规范化提交
git add .
git commit -m "feat(module): concise and professional description"

# 推送主分支 (触发 CI 与 Docker 构建)
git push origin main

# 打标发布新版本 (触发 GitHub Release 与多架构二进制打包)
git tag -fa v0.6.0 -m "Release v0.6.0: Next-Gen Architecture & Hardening"
git push origin v0.6.0
```


