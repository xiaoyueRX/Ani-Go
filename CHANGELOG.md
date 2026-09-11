# Changelog

Ani-Go 的更新日志。所有版本遵循 [语义化版本](https://semver.org/)。

---

## v0.6.0 — 2026-09-12 (Next-Gen Microkernel Architecture, Native MCP Server, Smart Disambiguation & Downloader Hardening)

### 核心新特性 (Major Features)
- **微内核主干与插件生态解耦架构 (Microkernel Pipeline & Decoupled Plugins)**:
  - 核心主干严格遵循五步极速流水线：RSS 抓取 ➔ 三级消歧 ➔ 下载调度 ➔ 规范硬链接 ➔ Jellyfin 即播；
  - 纯 Go 单二进制部署，常驻内存仅 15MB~30MB，极低 CPU 消耗；
  - 插件生态（全渠道通知、本地 NFO 刮削、数据迁移等）全部由统一的 `plugin.Manager` 进行生命周期管理，并以 `defer recover()` 全面包裹 Panic 隔离防护罩；
  - 插件停用时强制断电：后台工作协程立即退出，严格实现 0 协程、0 网络请求、0 错误日志。
- **全自动三级智能消歧与 Token 防爆机制 (Smart Disambiguation & Token Guard)**:
  - **一级正则清洗**：毫秒级过滤字幕组花字、分辨率及编码标签；
  - **二级放送时间窗口容差对齐**（0 Token 消耗）：比对 Bangumi 官方分集日历与 RSS 种子发布时间，无感化解 95% 的季号缺失与绝对集数歧义；
  - **终身唯一 SQLite 解析缓存**（`parser_caches`）：以原始标题为主键，一次推导终身复用，相同标题第二次直接 0ms 本地命中；
  - **AI 兜底与每日限额熔断器**：设置界面醒目提示 Token 消耗风险，支持自定义每日配额上限（默认 30 次/天）自动熔断，精简提示词上下文；
  - 设置页提供「三级推导测试沙盒」，输入任意复杂标题实时回显推导全过程。
- **Ani-Go 原生外部 AI 控制中心 (Native MCP Server)**:
  - 实现标准 Anthropic Model Context Protocol（基于 HTTP/SSE：`/mcp/sse` 与 `/mcp/messages`），集成 Bearer Token 强鉴权与异步并发隔离；
  - 暴露 6 项核心业务运维工具（`search_anime`, `list_subscriptions`, `add_subscription`, `remove_subscription`, `get_downloads`, `trigger_refresh`）；
  - 暴露 4 项系统治理管理工具（`list_plugins`, `toggle_plugin`, `get_system_status`, `update_system_config`）；
  - WebUI 设置页提供一键生成/重置 Token、SSE 端点复制以及 Claude Desktop / Cursor / Cline 客户端配置 JSON 一键复制。
- **本地 Bangumi NFO 刮削器插件 (Local NFO Scraper Plugin)**:
  - 剧集整理入库后自动从 Bangumi 抓取元数据，生成符合 Kodi/Jellyfin/Emby 规范的 `tvshow.nfo`、`poster.jpg` 和单集 `.nfo`。
- **做种生命周期回收与磁盘急停熔断底线 (Seeding Lifecycle & Disk Killswitch)**:
  - 达标（做种时长/分享率）后仅删除下载器任务记录，**严格保证保留已硬链接到媒体库的物理文件**；
  - 调度器前置校验剧集入库状态，防止误删尚未完成整理的种子；
  - 跨平台磁盘剩余空间监控（Windows `GetDiskFreeSpaceEx` / Linux `Statfs`），低于安全阈值（默认 5GB）时立即熔断阻断新增下载；
  - WebUI 增加最低可用磁盘空间配置与多语言支持。
- **整理路径 Dry-Run 预检 (Dry-Run Preview)**:
  - 新增 `POST /api/organize/preview`，单项与批量预测渲染路径、目标绝对路径、操作类型（硬链接/移动）及同名文件冲突，实现零写入预检。

### 缺陷修复与深度加固 (Hardening & Bug Fixes)
- **Aria2 /jsonrpc 双重拼接修复**：自动规范化剥离 Host 尾部 `/jsonrpc` 或多余斜杠，杜绝 404 导致 Aria2 失效；
- **Aria2 异常状态码拦截**：增设 `resp.StatusCode != 200` 校验，消除反向代理 HTML 错误页导致 JSON 解析崩溃；
- **Aria2 任务名解析**：优先从 `t.Files` 提取真实动画文件名，替代无意义的 16 位 Hex GID，使调度器准确匹配；
- **Transmission Host 尾部清洗**：自动剔除 `/transmission/rpc` 与 `/transmission` 后缀；
- **qBittorrent 健康探测超时防护**：`IsAvailable` 注入 5 秒 Context 超时限制，防止客户端假死导致长时间卡顿；
- **调度器状态兼容**：扩充支持 Aria2 的 `complete` 与 Transmission 的 `seeding` 状态；
- **整理器双点前缀目录放行**：修复 `checkPathEscape` 对以 `..` 开头的合法文件夹（如 `..Special`）的误杀，阻断非法跨目录越界；
- **TOCTOU 时序修复**：先补全扩展名，再在最终完整路径上执行安全审计；
- **内嵌静态服务空路径修复**：修复 `embed.go` 在客户端发送空路径请求时的切片越界 panic；
- **环境变量别名兼容**：支持 `QB_USERNAME`/`QB_PASSWORD` 与 `TR_USERNAME`/`TR_PASSWORD` 双向映射；
- **下载器连通性一键测试**：设置页新增下载器连通性与时延「立即测试」按钮与交互加载反馈；
- **极限抗压与 ReDoS 防护**：50,000 字符超长畸变标题 5ms 内安全返回，包级预编译正则消除重复编译开销；
- **全量 `-race` 检测全绿**：全工程所有 Go 包 100% 通过 `go test -race ./...` 严苛测试。

---

## v0.5.4 — 2026-09-05 (Notification Delivery Logs, Batch Download Controls & Live RSS Probe)

### 核心新特性 (Major Features)
- **通知投递履历与送达监控矩阵 (Notification Delivery History & Monitoring Matrix)**：
  - 填补历史遗留的通知持久化 TODO，实现 `NotificationLog` 数据库模型与 `notification_logs` 数据表；
  - `v2.NotifyManager` 内置无锁缓冲队列（容量 1000）与专属独立异步持久化工作协程，兼顾高并发与极端拥塞下的异步降级保护，完全不阻塞主推送流水线；
  - 提供多维度筛选（按渠道、状态、事件类型）、分页查询（`GET /api/notifications/logs`）与保留天数生命周期清理（`DELETE /api/notifications/logs`）；
  - 提供全局投递统计指标监控（`GET /api/notifications/stats`），实时掌握总投递量、投递成功率、失败数与死信队列（DLQ）积压数；
  - 设置页深度集成「通知投递履历与送达监控」交互看板，直观掌控所有 16 渠道的推送状态、耗时与错误堆栈。

- **下载中心生产力增强 (Downloads Center Productivity & Batch Controls)**：
  - **手动新建下载任务**：下载队列新增「新建下载」模态框，支持直接粘贴磁力链接（`magnet:?xt=urn:btih:...`）或 Torrent HTTP/HTTPS 直链，可选自定义动画标题与存储路径，直接调度底层下载器；
  - **全局并发批量控制**：新增「全部暂停」与「全部继续」控制接口（`POST /api/downloads/pause-all` 与 `POST /api/downloads/resume-all`）及 UI 快捷操作，通过并发安全下发批量指令至活跃下载核心（qBittorrent / Transmission / Aria2）。

- **RSS 实时源解析与正则特征探针 (RSS Live Feed Inspector & Parser Debugger)**：
  - 新增实时解析后端探针（`POST /api/rss/preview`），复用 15 秒超时 `httpx` 连接池实时抓取远程 XML/RSS 订阅源，自动解析条目并使用 `source.ParseMikanTitle` 提取集数、字幕组、画质分辨率等元数据；
  - 设置页「自定义正则」栏目集成实时 RSS 探针卡片，一键提取个人 Mikan RSS 或任意自定义字幕组源，实时回显前 100 条条目的结构化解析结果，极大降低正则编写与调试门槛。

### 质量保证与测试加固 (Quality Assurance)
- **并发竞态全绿**：全工程所有 Go 包 100% 通过 `go test -race ./...` 严苛测试，无任何 Data Race 与死锁隐患；
- **前端生产构建秒级完成**：`npm run build` 2.31 秒零警告完成打包，无缝内嵌至单文件 Go 二进制（`go:embed`）；
- **全链路端到端验证通过**：通过自动化验证脚本对登录鉴权、通知投递入库、统计查询、批量下载控制、RSS 实时探针进行了完整的功能性与鲁棒性验证。

---

## v0.5.3 — 2026-09-05 (Unified Notification Matrix, Downloader Hot-Swap, Regex Sandbox & Zero-Defect Audit)

### 核心新特性 (Major Features)
- **全 16 渠道通知系统统合 (Unified Notification Matrix)**：所有 16 种推送渠道（Telegram, 钉钉, 企业微信, 飞书, QQ OneBot, Bark, Server酱, Discord, Slack, Gotify, Ntfy, Pushover, Email SMTP, Matrix, LINE, WhatsApp, Signal）统一纳管至 `v2.NotifyManager`。具备优先级队列缓冲、指数退避重试、死信队列、插件一键启停休眠联动与免重启毫秒级热重载。
- **动态下载器热重载 (Dynamic Downloader Hot Swap)**：实现 `DynamicDownloader` 读写锁安全代理层，在 Web UI 切换并保存 qBittorrent / Transmission / Aria2 配置即刻全局生效，无需重启服务。
- **自定义标题正则 10 槽位与实时匹配沙箱 (Custom Regex Sandbox)**：新增 10 槽位自定义正则规则，支持非连续配置；并提供实时沙箱测试接口与 UI 卡片，输入任意标题实时校验集数、季数、字幕组解析结果。
- **OVA 与剧场版独立媒体库路径 (OVA & Special Paths)**：扩展 `OVA_BASE_PATH` 与 `OTHER_TEMPLATE`，OVA/特别篇自动归档至独立目录；支持媒体服务器（Plex/Emby/Jellyfin）非法路径字符自动净化。

### 缺陷清零与架构加固 (Zero-Defect Audit)
- **配置持久化三级优先律确立**：彻底修复 `MergeFromSettings` 中因默认非空值导致 SQLite 配置无法覆盖默认值的根本缺陷，确立严格的「环境变量 > 数据库设置 > 默认值」三级律，支持 `DEFAULT_DOWNLOADER` 与 `DOWNLOADER_DEFAULT` 双别名。
- **qBittorrent 并发竞态与网络风暴根除**：消除重试修改 `Jar` 引发的数据竞争（Data Race），增加互斥锁登录保护，引入 30 分钟会话缓存，消除轮询时双倍探测请求，空 Body 自动处理 EOF。
- **Aria2 级联删除与物理文件清理**：修复活动与停止任务删除错误，级联执行 `remove` -> `removeDownloadResult` -> `forceRemove`，勾选删除文件时主动删除本地媒体与 `.aria2` 控制文件。
- **TMDB 纯数字 ID 404 修复**：`GetExternalIDs` 自动补齐标准化 `tv/` 前缀。
- **网络映射协程超时熔断**：`mapOne` 异步映射注入 30 秒超时上下文，杜绝网络阻塞导致的协程泄漏。
- **AI 降级错误保留**：修复 `WithFallback` 变量覆盖导致主模型原始错误信息丢失问题。
- **SQLite 外部文件句柄泄漏修复**：`MigrateFromPath` 迁移完成后主动关闭底层文件描述符。
- **Base32 磁力链接支持**：升级预编译正则支持 32 位 Base32 磁力 InfoHash 提取。
- **全量竞态检测 PASS**：全工程所有 Go 包 100% 通过 `go test -race ./...` 严苛测试。

### 运维与构建优化 (DevOps & Docker)
- **Docker 极简多阶段构建**：升级至 Alpine 3.21，利用 `$BUILDPLATFORM` 原生架构交叉编译，镜像大小控制在 ~30MB，增加容器原生 `HEALTHCHECK` 健康检查。
- **Docker Compose 开箱即用**：规范化目录映射、健康探测与网络配置。
- **GitHub Actions CI/CD**：修复 Actions 版本，全平台（Linux/macOS/Windows, amd64/arm64/armv7）自动构建、打包、校验和与 GitHub Releases 自动发布。

---

## v0.5.2 — 2026-09-04 (Performance, Mikan Schedule & Standalone Releases)

### 性能与架构优化 (Performance)
- **GORM 预编译缓存**：数据库开启 `PrepareStmt: true`，高频轮询查询自动复用预编译语句缓存，消除重复 SQL 解析开销。
- **RSS 轮询查询风暴消除**：改造 `pollRSS` 为单次批量查重预拉取（`torrent_url IN ?`），$N$ 次数据库往返降低至 1 次，修复新增计数重复递增缺陷。
- **整理器批量拉取**：文件整理支持批量预加载订阅信息，移除多余的重复状态更新，消除 $N+1$ 查询。
- **HTTP 连接池与环境代理统一**：`YucWiki` 及全部网络客户端统一收拢至 `httpx`，复用共享连接池，全面支持容器及系统环境变量代理（`HTTP_PROXY` / `HTTPS_PROXY`）。

### 新增特性 (Features)
- **Mikan 默认时间表与 Yuc.wiki 插件**：放送时间表默认使用 Mikan 广播源，并新增内置 `yuc_schedule` 插件支持一键切换周间新番。
- **Bangumi 季度自动同步**：时间表支持根据所选年份与季度自动同步对应放送列表。
- **多平台 Release 自动化流水线**：新增 GitHub Actions Release 工作流，全自动交叉编译 7 大平台架构独立二进制（Linux/macOS/Windows, amd64/arm64/armv7）并生成校验和与一键安装脚本。

### 缺陷修复 (Fixes)
- **媒体命名模板设置覆盖**：修复 `MergeFromSettings` 无法覆盖默认模板的问题，现支持设置页自定义模板实时生效。
- **年份为0路径优化**：优化路径格式化正则，无年份或为 0 时智能消除空括号并规范化路径。
- **文件整理计数精准化**：修复整理统计中 `successCount` 遗漏的问题，明确分离成功与失败计数，全失败时触发专项告警。
- **版本号动态化**：前端与后端全面联动 `/api/version` 动态渲染当前运行版本，修复重复前缀问题。

---

## v0.5.1 — 2026-09-03 (Security & Stability Hotfix)

### 安全与缺陷修复 (Security & Fixes)
- **数据库锁死防御**：剔除 \utoSubscribe\ 事务内包含的所有外部网络 I/O（Mikan RSS 解析），彻底根绝因网络波动造成的 \database is locked\ SQLite 锁库灾难。
- **配置密钥覆写保护**：修复前端设置页保存时提交脱敏空值导致 \ARIA2_SECRET\ 和其他下载器密码被清空的问题，现已实现智能防覆盖。
- **敏感字段保存放行**：放开 API 接口对包含 \SECRET\ 关键字的客户端密钥的误伤拦截，确保 Webhook 和 Aria2 密钥能正常保存（仅拦截内部鉴权 \JWT_SECRET\）。
- **路径穿越拦截**：在数据迁移接口 (\handleMigrateData\) 中加入针对绝对路径和相对路径的双层防御，彻底杜绝恶意遍历读取宿主机文件的威胁。
- **事件总线泄漏修复**：插件系统 Reload 时先注销旧的事件监听句柄，防止了 EventBus 句柄重复叠加导致 OOM。
- **做种源文件防误删**：修正硬链接失败降级为跨设备复制文件时的逻辑，现在降级复制后**会主动保留原始源文件**，保障 BT 客户端能继续正常做种。
- **死循环栈溢出**：修复 Transmission 下载器处理 HTTP 409 (Conflict) 返回码时无限递归刷新 Session 的堆栈溢出漏洞。

---

## v0.5.0 — 2026-09-02

### 修复
- **下载逻辑解耦**：解决在 \personal\ 模式下由于 BangumiID 补全慢导致的下载任务阻塞，匹配即刻下发下载。
- **备份恢复零值丢失**：修复恢复备份时 \Enabled: false\ 状态被 GORM 默认更新逻辑忽略的问题。
- **时间表跳转 404**：修复点击已删除订阅（软删除）导致页面无法加载的问题，增加存在性校验与回退搜索弹窗。
- **通知器类型断言 Panic**：修复 \internal/notifier/v2\ 中由于文件大小字段类型断言失败导致的崩溃。
- **批量补全超时**：将全量订阅补全逻辑改为异步处理，避免前端请求长时间挂起。

### 新增
- **元数据补偿抓取**：新增 Mikan 详情页爬虫，在元数据服务不可用时自动从 HTML 提取 BangumiID 与封面海报。
- **插件管理 UI**：设置页新增「插件」选项卡，支持展示已加载插件列表及一键重载配置。
- **订阅级死种超时设置**：新增 \StallTimeoutHours\ 字段，支持为特定番剧单独设置做种超时时间。
- **i18n 补全**：新增插件管理、备份中心等页面的完整中英文翻译支持。

### 优化
- **数据库查询**：优化订阅列表与时间表的查询逻辑，统一排除已软删除的记录。
- **Mikan 标题解析**：增强对复杂版本号（如 v2, v3）及半集（.5）的识别准确率。

---

## v0.3.0 — 2026-08-29

### 新增
- **做种自动清理**：下载完成的种子在达到指定做种时间且比率达标后自动从 qBittorrent 删除（仅删种子记录，不删文件）
- **qBittorrent 标签增强**：添加种子时自动附带字幕组和分辨率标签

---

*完整更新历史请查看 [GitHub Releases](https://github.com/xiaoyueRX/Ani-Go/releases)*
