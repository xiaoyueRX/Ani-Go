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

### 5.5 完整测试验证
- Search: 4 results ✅
- Mikan Groups: 14 groups ✅  
- Health check 无 token 可访问 ✅
- Create subscription with BangumiID → async RSS resolution ✅
- Go vet: 0 warnings ✅
- TypeScript: 0 errors ✅
- Build: success ✅

## 待处理（网络恢复后执行）
```bash
cd /root/x/Ani-Go
git push origin main
```
4 pending commits: ddbf461, 0b2e86a, a290872, 6f1d8da

## Session 6: 剧集状态管理 + 登录页美化

### 6.1 剧集状态手动切换
- **新增**: `PUT /api/episodes/{id}/status` 后端端点
- **新增**: 前端点击状态 badge 循环切换 pending→downloading→completed→pending
- **效果**: 可直接在剧集列表中手动标记下载状态
- **文件**: `handlers.go`, `SubscriptionDetail.vue`

### 6.2 登录页版本信息
- **新增**: 底部版本号文字
- **文件**: `Login.vue`

### 6.3 最终推送成功
- 7 commits 全部推送到 GitHub（ddbf461 ~ c42c4be）
- GFW 在最后一次尝试时恢复连接

## Session 7: 严重 Bug 修复与图片防盗链突破

### 7.1 后端崩溃与调度器修复
- **问题**: `pollRSS` 在缺失 InfoHash 时导致数据库关联丢失与死循环。
- **修复**: 重构了 `pollRSS` 和 `pollDownloads` 逻辑，修复了 Mikan RSS 缺乏 Hash 时的平滑过渡；修复了空指针异常；重写了调度器逻辑。
- **文件**: `internal/scheduler/scheduler.go`

### 7.2 前端图片防盗链突破
- **问题**: Mikan, BGM.tv, Bilibili 开启防盗链，导致所有封面图无法加载。
- **修复**: 在前端所有组件 (`Search.vue`, `Schedule.vue`, `Subscriptions.vue`) 拦截所有外部图片，转发到后端的 `/api/proxy/image` 端点。后端端点智能伪装 `Referer` 头绕过拦截。
- **文件**: `internal/api/handlers.go`, `web/src/views/Search.vue`, `Schedule.vue`, `Subscriptions.vue`

### 7.3 订阅列表为空修复
- **问题**: 创建订阅时 API 未接收并保存 `CoverURL`，导致前端数据解析异常渲染为空列表。
- **修复**: 补全 `createSubscriptionRequest` 的字段。修改 `handleSchedule` 优先使用 Mikan 源获取更全的时间表数据并实现 BangumiID 准确绑定。
- **文件**: `internal/api/handlers.go`, `internal/source/multi.go`

## 总结

| Session | 改动量 | 主要内容 |
|---------|--------|---------|
| 1 | 5 files | 基础设施修复（health/docker/main.go） |
| 2 | 4 files | 搜索→订阅全流程（字幕组+RSS解析） |
| 3 | 8 files | UI美化全覆盖（IconSax/设置页重构） |
| 4 | 3 files | 搜索缓存+订阅列表筛选+自动刷新 |
| 5 | 5 files | 代码审查+Mikan Groups修复+搜索超时 |
| 6 | 4 files | 剧集状态切换+最终推送 |

## Session 6: 前端图标库迁移与后端健壮性修复

### 6.1 前端图标库迁移至 Lucide
- **重构**: 移除不再维护的 `IconSax`，全面迁移至 `lucide-vue-next`。
- **文件**: 删除了 `web/src/components/IconSax.vue`。
- **更新**: 修改了所有依赖图标的 Vue 组件 (`Layout.vue`, `Login.vue`, `Downloads.vue`, `Schedule.vue`, `Search.vue`, `SettingsPage.vue`, `SubscriptionDetail.vue`, `SubscriptionForm.vue`, `Subscriptions.vue` 等)，确保图标统一和 TypeScript 类型安全。
- **配置**: 更新了 `package.json` 依赖，重构了 `style.css`。

### 6.2 后端启动流程与健壮性修复
- **修复**: 在 `main.go` 中补充了对 `api.StartServer` 的错误捕获，若 HTTP API 服务因端口占用等原因启动失败，现在会通过 `log.Fatalf` 抛出明确的报错而不是静默失败或引发后续空指针。
- **修改文件**: `main.go`。

### 6.3 文档更新
- **修复**: 修正了 `AGENTS.md` 中关于前端代理的陈旧描述，明确 `vite.config.ts` 已经正确配置了 `/api` 的 Proxy，可直接代理到后端 `20001` 端口，提升开发体验。

### 6.4 后端 API 与调度器优化
- **修改**: 调整了 `internal/api/handlers.go`, `internal/api/server.go`, `internal/scheduler/scheduler.go`, `internal/source/mikan.go` 等核心逻辑，修复了遗留的逻辑异常和潜在空指针风险。
