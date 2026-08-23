# Changelog

Ani-Go 的更新日志。所有版本遵循 [语义化版本](https://semver.org/)。

---

## v1.4.0 — 2026-08-24

### 新增

#### 通知系统全面重构
- **v2 通知器适配器**：`internal/notifier/v2/adapters.go` 实现 Telegram/钉钉/企业微信/飞书/OneBot(QQ) 真实发送逻辑
  - Telegram：支持 MarkdownV2、特殊字符自动转义、MarkdownV2 解析模式
  - 钉钉：Webhook Markdown 消息、加签预留
  - 企业微信：Text 类型消息、@all 提及支持
  - 飞书：Post 富文本卡片消息
  - OneBot (QQ)：私聊/群聊双通道、Bearer Token 认证
- **通知管理器**：`internal/notifier/v2/manager.go` 企业级异步分发中心
  - Worker 池并发处理（默认 4 workers）
  - 指数退避重试策略（10s/20s/40s...，最多 3 次）
  - 死信队列 (DLQ) 持久化失败消息
  - 事件路由规则：按事件类型路由到指定渠道
  - 热重载：配置变更即时生效，无需重启
- **EventBus 深度集成**：订阅 5 大核心业务事件
  - `download.started` / `download.completed` / `file.organized` / `episode.missing` / `error`
- **测试通知 API**：`POST /api/notify/test` 支持单渠道/全渠道测试
- **WebUI 设置页**：notify 标签页各渠道字段新增 🔔 测试按钮，一键验证配置

#### 整理器增强
- 硬链接优先（同文件系统），跨设备自动回退复制+删除源文件
- 优先使用 qBittorrent `content_path`（已在最终目录），避免跨卷移动
- 文件存在性检查：跳过 `missingFiles` / 进度 < 1.0 的种子
- TMDB 刮削重命名：按 BangumiID 获取中文标题、年份、季数、集标题
- 命名模板：`{title_cn} ({year})/Season {season}/{title_en} S{season:02}E{ep:02}{ext}`

#### Mikan 个人 RSS 确认
- `RSS_MODE=personal`：仅下载已订阅番剧
- `RSS_MODE=classic`：自动建订阅（经典模式）
- 镜像域名自动测速回退：`mikanani.me` / `mikanime.tv` / `mikanani.kas.pub`

### 修复

- **整理器匹配逻辑**：三级匹配策略（hash前缀 → original_name精确 → title模糊），修复原 `|| ContentPath != ""` 导致的错误匹配
- **调度器 pollOrganizer**：增加文件存在性检查，正确跳过未完成/失败下载
- **跨设备移动失败**：`os.Rename` EXDEV 错误改为复制+删除，硬链接失败统一回退

### CI/CD

- Docker 多架构构建：`linux/amd64,linux/arm64` (GitHub Actions + QEMU + Buildx)
- 前端构建：Vite 8 + Vue 3 + TypeScript 6 + TailwindCSS v4 + DaisyUI v5
- 后端构建：Go 1.25 + CGO_ENABLED=0 纯 Go 部署

---

## v1.3.0 — 2026-05-30

### 新增

#### 国际化 (i18n)
- 中英文双语完整支持（`web/src/locales/zh.ts` + `en.ts`）
- `web/src/i18n.ts` 国际化引擎，语言切换即时生效
- 覆盖所有页面：设置、订阅、搜索、时间表、下载、登录

#### 新手引导与版本管理
- **OnboardingModal**：首次登录引导弹窗，帮助新用户快速上手
- **ChangelogModal**：更新日志弹窗，内置版本变更查看
- **useVersion composable**：版本检测 + 更新提醒

#### 批量操作 API
- `POST /api/subscriptions/batch`：批量创建订阅（支持多字幕组选择）
- `DELETE /api/subscriptions/batch`：批量删除订阅（可选删除文件）
- `POST /api/subscriptions/batch/restore`：批量恢复已删除订阅

#### 前端 Lucide 图标迁移
- 从 IconSax 迁移到 `lucide-vue-next`，统一图标风格
- 删除旧 `IconSax.vue` 组件，所有视图改用 Lucide 组件

#### 前端大幅重构
- **Schedule.vue** 重写：时间表页面全面重构（+864 行），支持按星期分组 + 海报图
- **Subscriptions.vue** 重构：订阅管理页面优化（+525 行），支持批量操作
- **SettingsPage.vue** 重构：设置页大幅改进（+557 行），i18n 集成
- **Search.vue** 重构：搜索页优化（+378 行），交互体验提升
- **SubscriptionDetail.vue** 重构：订阅详情页改进（+506 行）
- **Layout.vue** 重构：布局组件优化（+261 行）
- **Login.vue** 重构：登录页改进（+169 行）
- **Downloads.vue** 重构：下载页优化（+187 行）
- 新增 `SubscriptionCard.vue` 订阅卡片组件
- 新增 `SubscriptionEditForm.vue` 订阅编辑表单
- 新增 `ScheduleCard.vue` 时间表卡片组件
- 剧集 API 新增 `group_name` 字段（显示字幕组名称）

### 安全

- **CORS Origin 白名单**: 从配置文件读取允许的 Origin 列表，非 Debug 模式禁用通配符，防止跨域攻击
- **设置页密码脱敏**: 设置 API 返回时隐藏敏感密码字段，防止信息泄露

### 性能

- **Schedule API N+1 修复**: 批量 GROUP BY 查询替代逐条查询，时间表加载提速
- **搜索缓存 LRU 改造**: 引入 LRU 缓存（128 条上限）替代无限制 `sync.Map`，防止内存无限增长

### 可靠性

- **EventBus 订阅管理**: `Subscribe` 返回 `SubscriptionID`，`Unsubscribe` 通过 ID 精确移除，避免 goroutine 泄漏
- **EventBus 超时保护**: `Publish` 中 handler 执行加入 5s 超时，防止慢 handler 阻塞事件总线
- **Transmission 竞态修复**: tag 计数器改用 `sync/atomic`，消除并发竞态条件
- **MikanSource 读写锁**: domain 读写加入 `sync.RWMutex` 保护，解决并发数据竞争
- **JWT Secret 持久化**: 密钥持久化到 SQLite settings 表，重启不丢失（之前每次重启重新生成，导致已有 Token 失效）
- **MultiNotifier 错误聚合**: 用 `errors.Join` 聚合多平台发送错误，不再丢失错误信息
- **插件 Shell 审计日志**: Shell 插件执行增加 AUDIT 审计日志，便于安全审计
- **Logger 文件句柄**: 退出前关闭日志文件句柄，防止资源泄漏
- **QBittorrent Hash 修复**: 修复 `GetTorrentHashByURL` 逻辑错误
- **Mikan 空 Hash 唯一约束修复**: 修复空 torrent hash 导致的 unique constraint 失败
- **调度器死循环修复**: 修复调度器中潜在的无限循环问题
- **yucwiki 正则修复**: 修复 `~` 后有空格导致的匹配失败

### CI/CD

- **前端构建步骤修复**: CI 流水线前端构建步骤更新
- **新增 mikanani.kas.pub 镜像**: Mikan 镜像列表新增 `mikanani.kas.pub` 域名
- **测试修复**: nil pointer 修复 + 域名期望值更新 + 镜像数 2→3

---

## v1.2.0 — 2026-05-07

### 新增

#### 新番时间表（默认首页）
- 登录后默认打开「新番时间表」页面（原为订阅管理）
- **放送表**: 从 yuc.wiki 获取当前季度全部新番，按星期分组显示
- **我的订阅**: 已订阅番剧按放送日分组排列
- 数据源：yuc.wiki 日本动画时间表站（专业海报图，来自 Bilibili CDN）
- 自动按当前月份计算季度路径，最多回退 3 个季度
- 前端每 30 分钟自动刷新

#### 搜索 → 订阅全流程
- 搜索结果点击「订阅」弹出字幕组选择弹窗（DaisyUI modal）
- 调用 `GET /api/mikan/groups?bangumi_id=xxx` 获取可用字幕组
- 后台异步解析 BangumiID → RSS URL（若前端未选择）
- `createSubscriptionRequest` 新增 `rss_url` 字段

#### PWA 支持
- manifest.json：standalone 模式、主题色、图标
- service worker（sw.js）：安装即激活
- Chrome/Edge 可将网页安装为独立应用

#### 登录页「记住密码」
- 复选框控制是否保存账号密码到 localStorage
- 下次打开自动填充

#### 订阅列表搜索/筛选
- 搜索框：按标题/英文名/字幕组实时过滤
- 状态筛选按钮：全部/进行中/已完结

#### 剧集状态手动切换
- `PUT /api/episodes/{id}/status` 端点
- 点击剧集状态 badge 循环切换 pending→downloading→completed

#### Mikan 镜像测速
- 启动时自动并发测速所有镜像域名，选择延迟最低的作为主域名
- 设置页手动测速：显示各域名延迟（绿/黄/红色标识），点击可切换
- `POST /api/mikan/test-mirrors` 测速 API
- `POST /api/mikan/select-mirror` 手动选择 API（保存到数据库）
- 默认镜像新增 `mikanani.kas.pub`

### 修复

- **严重**: 调度器 `pollRSS` 和 `pollDownloads` 缺失 Hash 时死循环与空指针修复
- **前端图片防盗链**: 所有外部图片（Mikan, BGM.tv, Bilibili）通过后端 `/api/proxy/image` 伪装 Referer 绕过防盗链拦截
- **前端订阅列表空数据**: 修复 `createSubscriptionRequest` 未接收 `CoverURL` 导致列表渲染异常的 BUG，并优先使用 Mikan 源匹配
- `/api/health` 加入 AuthMiddleware 白名单，无需 token 可访问
- Mikan 中文搜索：`url.QueryEscape` 编码关键字解决 400 错误
- Mikan 搜索 CSS 选择器：备选 `a[href*="/Home/Bangumi/"]` 兼容新页面
- Mikan Groups 选择器：`data-anchor` 定位修正，14 个字幕组正常返回
- SubscriptionDetail 弹窗：改用 DaisyUI 标准 `showModal()` 方法
- main.go build 警告：`fmt.Println` → `fmt.Print`
- docker-compose.yml：填入有效 volume 示例路径

### 优化

- Mikan 搜索全局缓存（`sync.Map`，30s TTL，6x 提速）
- 搜索超时 25s + 耗时/时间显示
- 搜索失败提示优化（区分超时和错误）
- 移动端 UI 适配：按钮触摸区域、卡片内边距、网格响应式
- 侧栏标签横向/纵向自适应
- 登录页渐变背景 + 品牌图标

### 文档

- CLAUDE.md：新增 Phase 7 状态
- AGENTS.md：IconSax 组件说明
- 全套中英双语 docs/ 同步更新
- docs/WORK_LOG.md：完整开发日志

---

## v1.1.0 — 2026-05-06

### 新增

#### 前端 IconSax 图标系统
- `web/src/components/IconSax.vue` 组件，Iconsax Linear 风格 20+ 图标
- 全面替换所有视图的 inline SVG 和 emoji

#### 设置页重构
- 纵向侧边栏标签（原为横排）
- 区域分组卡片（下载器 4 组/通知 5 组/高级 2 组）
- 已配置状态 ✓ badge + 配置进度 3/5 指示
- 密码字段显隐切换

#### 搜索番剧页面
- `web/src/views/Search.vue`：Mikan + Nyaa/ACG.RIP/AnimeTosho 搜索
- 搜索结果卡片显示数据源、字幕组、大小等信息
- 搜索结果订阅功能

### 修复

- `core.TorrentItem` 添加 `json` 标签（`title`, `url`, `source` 等）
- 前端接口字段名匹配修复（`Title`→`title`）

### 优化

- 登录页：渐变背景、品牌图标容器
- 侧栏导航：Iconsax 图标、底部用户信息
- 订阅卡片：SVG 状态标识、hover 效果、进度条优化
- 下载列表：状态标签 SVG 图标
- 订阅详情：字段图标、状态 SVG

---

## v1.0.0 — 2026-04-29

初始版本发布。

### 核心功能

- Mikan RSS 自动追番 + 历史全量补全
- qBittorrent / Transmission / Aria2 多下载器
- AI 辅助（OpenAI/Google/Anthropic/Ollama）
- 自然语言任务解析器（正则 + AI 回退）
- 16 平台消息通知（Telegram/Discord/WeCom/飞书/钉钉/QQ/Slack/Matrix/LINE/WhatsApp/ServerChan/Bark/Pushover/Gotify/ntfy/Email）
- 插件系统（Webhook + Shell 脚本）
- Web UI（Vue3 + DaisyUI + JWT 鉴权）
- Docker 多阶段构建 + CI/CD（GitHub Actions 多架构镜像）
- AutoBangumi 数据迁移工具

### 文档

- 中英双语文档体系（README/AGENTS/CLAUDE/DEVELOPMENT_PLAN/PROJECT_CONTEXT/TRANSFER_CONTEXT）
