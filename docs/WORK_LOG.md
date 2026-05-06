# Ani-Go 工作日志

> 开始时间: 2026-05-06 ~17:00 (UTC) / 当地时间 ~02:00
> 结束时间: ~08:00
> 目标: 自动完善项目，持续 6 小时

---

## Session 1: 基础设施修复

### 1.1 `/api/health` 被 JWT 拦截
- **问题**: AuthMiddleware 只白名单了 `/api/login`，`/api/health` 被拦截
- **修复**: `internal/auth/middleware.go` — 白名单新增 `/api/health` 和 `/api/health/`
- **验证**: `curl /api/health` 返回 `{"status":"ok"}`

### 1.2 docker-compose.yml 空 volume 挂载
- **问题**: volumes 段只有空 `- `，无法直接使用
- **修复**: 填入示例路径 `./data:/data` 和 `/path/to/your/media:/TV/Media`

### 1.3 main.go build warning
- **问题**: `go vet` 报 `fmt.Println arg list ends with redundant newline`
- **修复**: `fmt.Println` → `fmt.Print`（原始字符串末尾自带换行）

## Session 2: 搜索→订阅全流程

### 2.1 Mikan 字幕组 RSS URL 解析 API
- **新增**: `GET /api/mikan/groups?bangumi_id=xxx`
- **新增**: `MikanSource.FetchSubgroups(ctx, bangumiID)` 爬取 Mikan 详情页提取所有字幕组名称 + RSS URL
- **新增**: `MikanSource.ResolveFirstRSSURL(ctx, bangumiID)` 获取第一个可用字幕组的 RSS URL
- **新增**: `source.SubgroupInfo` 结构体
- **文件**: `internal/source/mikan.go` + `internal/api/server.go` + `internal/api/handlers.go`

### 2.2 AuthMiddleware 白名单修复
- **问题**: health check 被中间件拦截
- **修复**: `internal/auth/middleware.go` — `/api/health` 加入放行列表

## Session 3: 前端增强

### 3.1 订阅列表搜索/筛选
- **新增**: 搜索输入框（按标题/英文名/字幕组过滤）
- **新增**: 状态筛选按钮（全部/进行中/已完结）
- **新增**: `filteredSubs` computed 属性
- **文件**: `web/src/views/Subscriptions.vue`

### 3.2 其他改进
- **修复**: 所有视图 `size="16"` → `:size="16"` 动态绑定（TypeScript 严格模式兼容）
- **文件**: `web/src/views/Downloads.vue` + `Layout.vue` + `Login.vue` + `Search.vue` + `SettingsPage.vue` + `SubscriptionDetail.vue` + `SubscriptionForm.vue` + `Subscriptions.vue`

---


## Session 4: 搜索→订阅字幕组选择 + 缓存

### 4.1 字幕组选择弹窗
- **新增**: 搜索页点击"订阅"弹出字幕组选择 modal
- **新增**: 调用 `GET /api/mikan/groups?bangumi_id=xxx` 获取字幕组列表
- **新增**: 用户选字幕组后传 `rss_url` 创建订阅
- **文件**: `web/src/views/Search.vue`, `internal/api/handlers.go`

### 4.2 RSS URL 自动解析
- **新增**: 创建订阅时若提供 `BangumiID` 但无 `RSS URL`，后台异步自动解析
- **新增**: `createSubscriptionRequest` 增加 `rss_url` 字段
- **文件**: `internal/api/handlers.go`

### 4.3 Mikan 搜索缓存
- **新增**: 全局 `sync.Map` 搜索缓存，30s TTL，跨请求共享
- **效果**: 第一次搜索 ~2.8s → 缓存命中 ~0.47s（~6x 提速）
- **文件**: `internal/source/mikan.go`

## Session 5: 代码审查与持续完善

### 5.1 订阅列表自动刷新
- **新增**: 每 30s `setInterval` 自动轮询订阅列表
- **新增**: `onUnmounted` 清理定时器
- **文件**: `web/src/views/Subscriptions.vue`

### 5.2 全量代码审查
- Go vet: 零警告
- TypeScript: 零错误
- Go build: 构建成功
- 审查范围: 所有新增/修改的 10+ 文件

### 5.3 Mikan Groups 修复
- **问题**: `leftbar-item` 是 `<li>` 元素，`.mikan-rss` 不在其内部
- **修复**: 改用 `data-anchor` 属性定位对应区块，再在区块内找 `.mikan-rss`
- **效果**: 14 个字幕组正常返回
- **文件**: `internal/source/mikan.go`

### 5.4 搜索超时 + 耗时显示
- **新增**: 搜索请求 25s 超时，超时提示
- **新增**: 搜索结果耗时和搜索时间显示
- **文件**: `web/src/views/Search.vue`

### 待处理
- GitHub push: GFW 阻断，3 commits 已本地保存
