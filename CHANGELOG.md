# Changelog

Ani-Go 的更新日志。所有版本遵循 [语义化版本](https://semver.org/)。

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
