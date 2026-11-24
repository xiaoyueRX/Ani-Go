# Ani-Go HTTP API

本文对应当前源码。部署实例的实际版本由 `GET /api/health` 返回。端点索引按功能列出请求方法、路径和鉴权；请求与响应的字段定义见 [OpenAPI 3.0](api.yaml)，路由注册见 [服务入口](../internal/api/server.go)。

## 登录与认证

首次启动会创建管理员。使用预置的 `ANIGO_ADMIN_PASSWORD` 或启动控制台输出的实例密码登录。以下 Bash 示例从终端读取密码，继续用同一会话的 JWT 查询插件。

```bash
base=http://localhost:20001
read -rsp 'Ani-Go 管理员密码: ' password
echo
jwt=$(printf '%s' "$password" | jq -Rs '{username:"admin",password:.}' |
  curl -fsS -H 'Content-Type: application/json' --data-binary @- "$base/api/login" |
  jq -er '.token')
unset password
curl -fsS -H "Authorization: Bearer $jwt" "$base/api/plugins"
```

一般管理请求发送 `Authorization: Bearer <JWT>`，登录有效期为 24 小时。签名密钥保存在本机数据库，正常重启会保留有效会话。修改管理员密码会使旧会话失效，并发修改只有一次能提交。账户存储暂时不可用时，登录与会话查询返回 503，可在恢复后重试。MCP 使用独立的访问令牌；115 或第三方面板的服务凭据只用于对应服务。头像读取、图片代理、健康检查和 Bangumi 授权回调有公开入口，详见下表。

## 端点总览

### 认证与用户

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `POST` | `/api/login` | 公开 | 登录获取 JWT Token |
| `GET` | `/api/me` | JWT | 当前用户信息 |
| `POST` | `/api/user/change-password` | JWT | 修改当前用户密码 |
| `POST` | `/api/user/avatar` | JWT | 上传 `avatar` 表单文件（PNG、JPEG、WebP、GIF 或 SVG，最多 5 MiB）；超限返回 413 |
| `GET` | `/api/user/avatar` | 公开 | 读取用户头像，保留旧 URL 兼容 |

### 健康检查与版本

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/health` | 公开 | 服务健康检查 |
| `GET` | `/api/version` | JWT | 版本号与内置更新日志 |
| `GET` | `/api/system/update/check` | JWT | 检查版本与部署环境的更新能力 |
| `GET` | `/api/system/update/progress` | JWT | 查询已启动的更新任务 |
| `POST` | `/api/system/update/execute` | JWT | 提交当前实例的更新任务 |

### 订阅与剧集

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/subscriptions` | JWT | 订阅列表（支持 `enabled` / `completed` 过滤） |
| `POST` | `/api/subscriptions` | JWT | 创建订阅 |
| `POST` | `/api/subscriptions/batch` | JWT | 批量创建订阅（最多 20 部） |
| `GET` | `/api/subgroups` | JWT | 已有字幕组去重列表 |
| `GET` | `/api/subscriptions/{id}` | JWT | 订阅详情 + 剧集列表 |
| `GET` | `/api/subscriptions/{id}/download-tasks` | JWT | 核对订阅中等待下载或整理的已绑定下载任务 |
| `PUT` | `/api/subscriptions/{id}` | JWT | 部分更新订阅。省略 `custom_path` 保留指定下载目录，空字符串或 `null` 恢复下载器默认目录。 |
| `DELETE` | `/api/subscriptions/{id}` | JWT | 删除订阅（`?delete_files=` 可选） |
| `POST` | `/api/subscriptions/batch-delete` | JWT | 批量删除（最多 100 部） |
| `POST` | `/api/subscriptions/batch-restore` | JWT | 批量恢复软删除订阅 |
| `POST` | `/api/subscriptions/{id}/trigger-supplement` | JWT | 触发单个订阅历史补全 |
| `POST` | `/api/subscriptions/supplement-all` | JWT | 异步触发全部订阅补全（预过滤后立即返回） |
| `POST` | `/api/subscriptions/merge-collections` | JWT | 按合集归并订阅元数据 |
| `POST` | `/api/subscriptions/{id}/resolve-collection` | JWT | 解析单个订阅所属的合集 |
| `POST` | `/api/subscriptions/{id}/verify-files` | JWT | 手动只读核对该订阅所选版本的库文件 |
| `POST` | `/api/subscriptions/{id}/episodes/{episodeID}/restore-file` | JWT | 从已完成下载的同卷原件补回指定版本的硬链接 |
| `POST` | `/api/subscriptions/{id}/episodes/batch-delete` | JWT | 删除当前订阅下选中的剧集 |
| `PUT` | `/api/episodes/{id}/status` | JWT | 设置或清除剧集的手动标记 |
| `POST` | `/api/episodes/{id}/retry-organize` | JWT | 将待复核文件重新排入整理队列，可附上核实的季集 |

### 下载队列

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/downloads` | JWT | 下载队列总览 |
| `POST` | `/api/downloads` | JWT | 手动新建下载任务（磁力链接 / Torrent URL） |
| `POST` | `/api/downloads/pause-all` | JWT | 全部暂停 |
| `POST` | `/api/downloads/resume-all` | JWT | 全部继续 |
| `DELETE` | `/api/downloads/{hash}` | JWT | 删除任务（可选同时删除文件） |
| `POST` | `/api/downloads/{hash}/pause` | JWT | 暂停单个任务 |
| `POST` | `/api/downloads/{hash}/resume` | JWT | 继续单个任务 |
| `POST` | `/api/downloader/test` | JWT | 下载器连通性与时延测试。空 JSON 对象测试当前实例。指定类型与地址时，传 `use_saved_credentials: true` 可在同一地址和账号使用已保存的凭据。Aria2 只匹配地址 |

### 设置、日志与自定义正则

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/settings` | JWT | 读取已保存设置、屏蔽凭据值与内部状态 |
| `PUT` | `/api/settings` | JWT | 批量 Upsert 设置 |
| `GET` | `/api/settings/custom-regex` | JWT | 当前自定义正则规则 |
| `POST` | `/api/settings/custom-regex/reload` | JWT | 从数据库重载自定义正则 |
| `POST` | `/api/settings/custom-regex/test` | JWT | 正则匹配沙箱（实时回显解析结果） |
| `GET` | `/api/logs` | JWT | 系统日志（`?lines=`，默认 100，上限 500） |

### 备份与恢复

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/backup/list` | JWT | 备份文件列表 |
| `POST` | `/api/backup/create` | JWT | 创建管理员 JSON 备份 |
| `POST` | `/api/backup/restore` | JWT | 从指定备份恢复设置和订阅，随后重载插件 |
| `DELETE` | `/api/backup/{name}` | JWT | 删除指定备份 |
| `GET` | `/api/backup/download/{name}` | JWT | 下载备份文件 |

### 通知投递监控

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `POST` | `/api/notify/test` | JWT | 指定渠道连通性测试 |
| `GET` | `/api/notifications/logs` | JWT | 投递履历（按渠道/状态/事件筛选 + 分页） |
| `DELETE` | `/api/notifications/logs` | JWT | 正整数 `days` 清理更早的记录；省略或传 0 永久清空全部，非法参数返回 400 |
| `GET` | `/api/notifications/stats` | JWT | 投递总数、成功数、失败数、死信数与渠道分布 |
| `GET` | `/api/notifications/templates` | JWT | 列出通知模板 |
| `PUT` | `/api/notifications/templates` | JWT | 保存指定事件与渠道的模板 |
| `POST` | `/api/notifications/templates/preview` | JWT | 预览模板渲染结果 |
| `POST` | `/api/notifications/templates/reset` | JWT | 重置指定 `event_type` 与可选 `channel` 的模板，缺少事件返回 400 |

### 解析与整理预检

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `POST` | `/api/rss/preview` | JWT | RSS 实时探针：抓取远程源并回显解析结果 |
| `POST` | `/api/organize/preview` | JWT | 整理 Dry-Run：预测目标路径、操作类型与冲突（零写入） |

### 搜索与发现

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/search` | JWT | 聚合搜索番剧资源 |
| `GET` | `/api/search/capabilities` | JWT | 查询智能搜索可用性 |
| `POST` | `/api/search/smart` | JWT | 智能搜索（搜索词扩展 + 结果分页） |
| `GET` | `/api/mikan/groups` | JWT | 按 BangumiID 获取字幕组与 RSS 地址 |

### 时间表与 Bangumi 集成

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/schedule` | JWT | 季度新番时间表（`source=mikan` / `yuc`） |
| `GET` | `/api/schedule/bangumi` | JWT | Bangumi 每日放送日历 |
| `GET` | `/api/bangumi/subject/{id}` | JWT | Bangumi 条目详情 |
| `GET` | `/api/bangumi/auth/link` | JWT | 生成 Bangumi OAuth 授权链接 |
| `GET` | `/api/bangumi/auth/callback` | 公开 | Bangumi OAuth 回调 |

### 镜像测速与 AI

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `POST` | `/api/mikan/test-mirrors` | JWT | Mikan 镜像测速 |
| `POST` | `/api/bgm/test-mirrors` | JWT | Bangumi 镜像测速 |
| `POST` | `/api/mikan/select-mirror` | JWT | 选定 Mikan 镜像并持久化 |
| `POST` | `/api/bgm/select-mirror` | JWT | 选定 Bangumi 镜像并持久化 |
| `POST` | `/api/ai/models` | JWT | 拉取已配置 AI 端点的可用模型列表 |

### 插件管理

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/plugins` | JWT | 插件总览（启用状态与关注事件） |
| `POST` | `/api/plugins/toggle` | JWT | 启用/停用插件 |
| `POST` | `/api/plugins/{id}/toggle` | JWT | 由路径指定目标的启停入口 |
| `GET` | `/api/plugins/custom` | JWT | 列出可导出的自定义清单，省略共享密钥 |
| `POST` | `/api/plugins/custom` | JWT | 保存单个自定义清单，与 `/api/plugins/save` 相同 |
| `POST` | `/api/plugins/save` | JWT | 新增或更新自定义插件 |
| `DELETE` | `/api/plugins/{id}` | JWT | 删除自定义插件 |
| `POST` | `/api/plugins/reload` | JWT | 重载插件配置 |
| `GET` / `PUT` / `DELETE` | `/api/plugins/{id}/panel/credential` | JWT | 查询凭据状态、保存或清除本插件的面板凭据 |
| `POST` | `/api/plugins/{id}/panel/actions/{action}` | JWT | 执行当前插件声明的动作，返回消息或分页任务 |
| `GET` | `/api/plugins/cloud-115/config` | JWT | 查看 115 令牌配置状态与来源 |
| `PUT` | `/api/plugins/cloud-115/config` | JWT | 保存或替换 115 OAuth 访问令牌 |
| `DELETE` | `/api/plugins/cloud-115/config` | JWT | 清除保存的 115 访问令牌 |
| `POST` | `/api/plugins/cloud-115/tasks` | JWT | 向 115 提交一条磁力链接 |
| `GET` | `/api/plugins/cloud-115/tasks` | JWT | 按页查询 115 账号的云下载任务 |

### 任务解析与数据迁移

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `POST` | `/api/parse` | JWT | 自然语言解析订阅任务（正则 + AI 回退） |
| `POST` | `/api/migrate` | JWT | 导入 AutoBangumi SQLite、ani-rss 订阅 JSON 或备份 ZIP |

### 图片代理与实时事件

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/proxy/image` | 公开 | 图片代理（域名白名单，绕过防盗链） |
| `GET` | `/api/events/stream` | JWT（可用 `?token=`） | SSE 实时事件流 |

### MCP 协议端点

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/mcp/sse` | MCP Token | MCP SSE 握手 |
| `POST` | `/mcp/messages` | MCP Token | MCP JSON-RPC 消息通道（`?session_id=`） |
| `POST` | `/api/mcp/token/generate` | JWT | 生成/重置 MCP Token |

## 订阅与剧集

使用 `POST /api/subscriptions` 手动添加订阅，至少提供 `title_cn`，可附上 `season`、`rss_url` 和字幕组限制。成功返回 201。同标题同季返回 409，同标题可分别创建不同季。省略季号或填 0 按第一季创建，旧数据中空或零季号也按第一季查重。TMDB 电视作品的各季共享作品 ID，因此同一 ID 可分别订阅不同季。已知类型的电影按影片查重；即使编号相同，电影和电视剧也可以分别订阅。相同蜜柑 ID、其他同一提供方的相同元数据 ID，或仅给出旧 `bangumi_id` 且该编号已存在时，也按重复订阅处理。蜜柑条目 ID 放在 `mikan_id`，外部元数据标识放在 `metadata_id` 并指定 `metadata_provider`。只有已知来源时再填提供方。详情包含按季集排序的剧集。部分更新时只发送改动字段。修改片名、季号或来源标识若与另一订阅冲突则返回 409，原记录保留。[订阅处理器](../internal/api/handlers.go)

通过 `PUT /api/subscriptions/{id}` 把 TMDB 电影订阅改为电视剧时，可同时发送 `{"metadata_id":"tv/456","anime_type":"TV"}`。明确的分类与 `movie/` 或 `tv/` 前缀冲突时返回 400。目标电影或目标电视剧的同一季已有订阅时返回 409。两种情况都会保留原记录。

```bash
curl -fsS -H "Authorization: Bearer $jwt" -H 'Content-Type: application/json' \
  --data '{"title_cn":"示例节目","season":1}' "$base/api/subscriptions"
```

- 搜索发布标题可向 `POST /api/settings/custom-regex/test` 发送 `{"title":"[CityLab] Harbor Story - 01 [1080p]"}` 预览作品名、季集、字幕组和分辨率。该接口只解析输入，适合在创建订阅前供用户校正结果。
- 同时启用同名多季时，共享 RSS 及历史搜索遇到无法区分季的发布会跳过。资源标题中的明确季标、对应的蜜柑作品 ID 或分季独立的 RSS 可区分归属。
- `current_episodes` 统计订阅季中已整理的正片集号。已知总集数时只计范围内的整数集号。同集多个版本只计一次，独立的手动标记保存在 `manual_status`。
- 当电影订阅的总数为 1 时，已确认入库的无集号正片也计为一部。同一电影的多个发行版本各自保留，进度仍为一部。待复核的视频和未确认的预告片不计入。订阅启用期间，达到 `1/1` 后仍会接收符合过滤条件的新 RSS 发布。
- 升级旧库或恢复旧 JSON 时，程序用已保存的正片确认信息或可核对的独立下载记录补算电影进度。恢复 JSON 会保留归档中的完结开关，因此读到 `1/1` 时也可能显示未完结。下一次自动补全扫描会按已启用订阅的实际进度刷新开关。
- 修改 `season` 或 `total_episodes` 后，更新响应即包含重新计算的进度。总集数已知时同时更新完结状态，总集数未知时保留原状态。同一次更新显式传入的 `completed` 优先。
- RSS 下载初期，剧集的 `file_size` 可来自订阅源。下载器报告任务大小后，该值更新为任务字节数。整理时绑定到具体文件后，每条记录保留各自的文件大小。
- 批量创建发送 `{"items":[…]}`，每批 1–20 项，各项可传 `season`，和单条创建采用相同的判重条件。按项读取 `success` 和 `failed`。单条或全量补全异步执行，202 表示已受理。全量补全最多同时处理三个订阅，同一订阅已有扫描时共享该次结果。
- 批量删除与恢复订阅发送 `{"ids":[1,2]}`。删除剧集向所属订阅的 `episodes/batch-delete` 发送 `{"ids":[1],"delete_files":false}`，单次 1–1000 个剧集 ID。跨订阅选择返回 404。删除共享下载的文件时会返回 409；下载器执行失败返回 502。[删除与文件归属](../internal/api/episodes.go)
- 写入手动标记时发送 `{"status":"completed"}`，可用值还有 `pending`、`downloading`、`failed`。发送 `{"status":""}` 可清除。成功响应中的 `status` 和 `manual_status` 均回显本次标记。读取订阅详情时，剧集的 `status` 是实际下载或整理状态，`manual_status` 是用户标记。整理进度和完结统计按实际状态计算。

订阅详情打开时会通过 `GET /api/subscriptions/{id}/download-tasks` 核对状态为 `downloading` 或 `downloaded` 的已绑定任务；同种子的多个成员只查一次。返回的 `tasks` 对象以小写任务标识为键。值为 `present`（找到）、`missing`（当前下载器已确认没有该任务）或 `unknown`（任务标识或队列信息不足）。`502` 表示下载器连接或响应异常，`503` 表示当前下载器不可用。此检查只读；要恢复缺失项，请在下载器中使用原种子、原目录与选集校验已有文件，然后再次核对。

The read-only download task check returns one result per bound task in the subscription. A `missing` result refers to the current downloader. `unknown`, HTTP 502 and HTTP 503 leave presence unconfirmed. To recover an original task, restore its torrent, directory and file selection in the downloader, verify the files, then check again.

通过 `POST /api/subscriptions/{id}/verify-files` 发送 `{"ids":[12,18]}`，一次选择 1–100 条当前订阅记录。重复 ID 按首次出现次序返回一条结果。未知、已删除或其它订阅的 ID 使整批返回 404。请求只接受 ID；服务器根据当前媒体库设置和已保存的最终路径核对文件，不读取媒体内容。响应示例：

```json
{"volume_guarded":false,"results":[{"id":12,"status":"size_matches"},{"id":18,"status":"missing"}]}
```

`size_matches` 表示普通文件存在且大小与逐文件证据相符；`size_mismatch` 包含 `expected_size`、`actual_size`。没有可靠历史大小时返回 `present`；这时若文件为零字节则返回 `empty`。当前库中未找到为 `missing`。库离线、配置卷标错误、没有权限或历史路径不能安全定位时返回 `unverified`，用固定的 `reason` 说明。未整理的记录为 `not_applicable`。`volume_guarded` 说明该媒体类型是否配置本机卷标。核对结果只适用于查询当时，人工标记、任务、记录和进度保持原值。[卷标配置与操作](05_DEPLOYMENT.md)

若核对发现单个已整理版本缺失，调用该版本的 `restore-file` 并发送 `{}`。未配置卷标识时，先确认当前挂载和目标文件名，再发送 `{"confirm_unmarked_volume":true}`。服务器向下载器核验已选且完成的原任务成员，按记录的大小、修改时间和原路径补回硬链接。原库父目录须仍在，原件与库目录须支持同卷硬链接。成功返回 `{"restored":true}`，人工标记和入库进度保持原值。原链接已存在且指向同一原件时，可以安全重试。其他内容占用目标、卷不可用、跨卷或来源证据不足时返回 409，并附上原因。

待复核条目的实际状态为 `needs_review`。详情给出 `review_reason` 和可执行操作。以下三种请求均使用 `POST /api/episodes/{id}/retry-organize`。成功返回 `{"status":"downloaded"}` 并由调度器重新核验。文件证据过期或记录状态已改变时，按 409 响应中的原因刷新条目。

| 详情中的可执行操作 | 请求体 | 用途 |
| --- | --- | --- |
| `can_retry_organize` | 空请求体 | 修正路径、挂载或文件后重新核验。 |
| `can_assign_episode` | `{"season":0,"number":15}` | 核对 `source_file` 后确认实际季集。季号 0 表示特别篇。 |
| `can_confirm_movie` | `{"confirm_movie":true}` | 确认该文件为电影正片。一个下载任务有多个版本时，每个版本可分别确认。 |

## 下载队列与媒体整理

`GET /api/downloads` 返回数组，空队列为 `[]`。下载器未配置返回 503，连接或读取失败返回 502。每项保留下载器原始 `status`，跨客户端显示使用 `display_status`；用 `can_pause`、`can_resume` 决定可执行操作。`complete` 表示下载完成，`progress` 为 0–1 的进度，`size` 和 `done` 的单位为字节。完成下载、整理入库是不同的状态，最终路径和剧集状态从订阅详情读取。[下载完成判定](../internal/core/download_task.go)

手动创建下载至少传 `url` 或 `magnet`。批量暂停与继续使用各自端点。可先向 `/api/organize/preview` 提交单项或批量样例，读取预测路径与冲突，再决定实际整理策略。[整理预览](../internal/organizer/preview.go)

## 设置与管理员备份

`GET /api/settings` 返回数据库已保存的键值，不代表全部进程环境。敏感值会屏蔽，插件清单、开关和社区面板凭据由插件专用接口管理。保存普通设置的请求格式是 `{"settings":{"RSS_INTERVAL":"15m"}}`。下载器配置实际改变时才会切换实例。通知与正则也有各自的重载行为。[配置处理](../internal/api/settings.go)

管理员 JSON 备份用于恢复本实例的配置和订阅，可含凭据，区别于可分发的插件清单。恢复含插件清单的快照会同步其面板授权并重载运行实例。未声明插件的旧快照保留本机面板授权。还原快照中的专有凭据时，内置面板以实际安装清单为准，115 保留原有键和环境变量优先级。若数据库已恢复而重载失败，接口返回 503，旧实例停止，待配置修复后可使用插件重载入口恢复。事务回滚则保留原实例。[备份实现](../internal/backup/backup.go)

新建备份保留原有顶层字段，并写入 `format_version: 1` 和 `sha256`。恢复先核对设置、订阅及剧集的摘要和记录归属。无摘要的历史快照继续按旧格式读取。文件损坏、版本不支持或记录归属不明确时返回 HTTP 400，文件不存在时返回 404。恢复不会更换本机 JWT 签名密钥。可以重复恢复同一快照，目标实例中的其他订阅会保留。

跨系统恢复时，已核实的合集成员按同一任务内的相对路径匹配。若 Windows 无法表示成员文件名中的字面反斜杠，接口返回 HTTP 400，指明源剧集记录并回滚本次恢复。[跨主机恢复步骤](05_DEPLOYMENT.md#在另一台主机恢复-json-备份)

定时和手工创建共用 `BACKUP_KEEP_COUNT`，默认为 7，成功后按时间淘汰旧文件。`BACKUP_CRON` 支持五字段、带秒六字段和 `@daily` 等标准别名。设置接口接受正整数保留数和有效表达式，保存后运行中的备份任务会读取新值。部署环境中的同名变量优先。[定时调度](../internal/backup/schedule.go)

## 10. 插件管理

`GET /api/plugins` 返回当前插件及运行状态。服务端判定 `origin` 为 `official` 或 `community`。`is_builtin` 说明分发方式，`author` 保留署名。`enabled` 是用户开关，`status` 可为 `inactive`、`loading`、`active`、`suspended`、`error`、`disposing`。已启用的插件缺少服务时，`missing_dependencies` 列出缺项，服务恢复后会自动运行。

| 操作 | 约定 |
| --- | --- |
| 保存或更新 | 向 `POST /api/plugins/save` 或 `POST /api/plugins/custom` 发送单份 Webhook 清单。读取回执 `id`、`enabled` 和 `status`。省略 ID 时以服务端分配值为准。更新已有清单保留原开关。保留 ID 冲突返回 409。 |
| 启停 | 向 `POST /api/plugins/toggle` 发送 `{"id":"cloud-115","enabled":true}`，或使用路径中的 ID 并发送 `{"enabled":true}`。未知 ID 返回 404。 |
| 导出、删除与重载 | 自定义列表省略共享密钥。删除自定义插件一并清理其开关和面板凭据。重载成功返回 `message`、`count` 和 `plugins`。 |
| 面板凭据 | 对 `/api/plugins/{id}/panel/credential` 使用 GET、PUT、DELETE。写入格式为 `{"access_token":"<服务令牌>"}`。读取只返回 `configured` 与 `source`。 |
| 面板动作 | 向 `/api/plugins/{id}/panel/actions/{action}` 提交 `{"input":"…"}` 或任务列表查询 `{"page":2}`。回执为文字消息或含 `page`、`page_count`、`tasks` 的分页结果。 |

搜索结果中的磁力动作另传 `placement:"search_result"` 与 `expected_url`。社区目标是用户确认的清单完整地址，内置插件使用空字符串。两项必须同时出现，输入错误返回 400，已变更的目标或入口返回 409。

未找到插件、面板或声明的动作、凭据时返回 404。插件未运行或缺必需凭据返回 409。管理器未初始化返回 503。服务授权失败返回 422，限流返回 429，上游未确认或回执不符合协议返回 502。兼容的面板协议版本为 1。完整字段、事件格式和作者服务端收发示例见 [插件作者指南](02_PLUGIN_SPEC.md) 和 [示例清单](../examples/plugins/cloud-tasks.json)。

### 115 云下载兼容接口

先在「设置 → 插件与自动化」启用官方 115 插件。旧接口 `/api/plugins/cloud-115/config` 与通用面板读取同一份授权。部署环境配置 `CLOUD115_ACCESS_TOKEN` 时返回 `source:"environment"`，从网页替换或清除会返回 409。省略环境变量时可在页面保存具备云下载权限的 115 OAuth 令牌。

`POST /api/plugins/cloud-115/tasks` 接收 `{"magnet":"<完整磁力链接>"}`，115 受理后返回 201 和 `{"submitted":true}`。用 `GET /api/plugins/cloud-115/tasks?page=1` 查看状态与 0–100 的整数百分比。提交结果不明时先查询列表。订阅下载和本地入库使用已配置的下载器。[115 适配器](../internal/cloud115/client.go)

## 搜索、迁移与实时消息

- 搜索返回的 `url` 和 `magnet` 用于下载。资源源同时给出普通下载地址和可用磁力时，`alternate_magnet` 供复制或发送到插件使用。只有磁力入口需要这个备用地址。
- `GET /api/search/capabilities` 的 `smart_search` 反映当前运行服务能否智能搜索。时间表可按年份、季度和来源查询，使用 Yuc 需要相应插件运行。Bangumi 条目路径需携带条目 ID。
- `POST /api/migrate` 需运行数据迁移插件。文件上传使用 multipart 字段 `file`，支持 AutoBangumi SQLite、ani-rss JSON 或其 ZIP 备份，文件上限 50 MiB。与 Ani-Go 自身的备份恢复使用不同入口。[迁移实现](../internal/migrate)
- 浏览器通过 `/api/events/stream` 订阅 SSE。MCP 客户端先连接根路径 `/mcp/sse`，读取 `endpoint` 事件，再向该地址发送 JSON-RPC；HTTP 202 表示服务器接收消息，结果通过 SSE 返回。通过 `tools/list` 查看系统工具及参数。其中 `trigger_refresh` 为已启用的订阅安排历史补全。[MCP 协议](03_MCP_SPEC.md)
