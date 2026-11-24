# Ani-Go 插件开发规范与架构白皮书

[English](02_PLUGIN_SPEC_EN.md) · [完整 JSON 示例](../examples/plugins/cloud-tasks.json) · [插件 API](API.md#10-插件管理)

本文描述当前源码的插件接口。部署时可用 `GET /api/health` 查看实际运行版本，构建默认版本来自 `main.go`。旧版部署须升级到包含声明式面板的构建后才能使用本文的表单与动作接口。[版本定义](../main.go)

## 1. 选择扩展方式

| 要做的事 | 如何交付 | 谁运行代码 |
| --- | --- | --- |
| 接收下载或整理事件 | 导入 `type: "webhook"` 清单，列出 `events` | Ani-Go 推送 JSON，作者部署的服务处理事件 |
| 为用户提供凭据输入、按钮和任务列表 | 在 Webhook 清单中加入 `panel`，实现一个公开 HTTPS 端点 | Ani-Go 渲染表单、验证输入、调用端点，远端服务执行业务 |
| 接入本地事件、服务或专用协议 | 实现 Go 的 `BuiltInPlugin` 并随 Ani-Go 编译 | 内置代码与 Ani-Go 在同一进程运行 |

`origin` 由服务端判定。`official` 表示列入 Ani-Go 项目官方清单的内置实现，`community` 包括导入的 Webhook 和未列入官方清单的内置实现。`is_builtin` 说明分发方式，`author` 和 `author_url` 用于署名。导入文件使用社区来源，不能覆盖内置 ID 或通过作者名取得官方标识。[来源与保留 ID](../internal/plugin/provenance.go)

例如，`cloud-115` 是 xiaoyueRX 维护的 Ani-Go 官方内置插件，实现的是 115 开放平台协议。NaJue 在原帖提出了提交磁力链接的需求。下文的 `cloud-tasks-example` 是独立的第三方服务示例，作者自行选择后端；向其提供的凭据由该服务接收。[官方插件用法](../README.md#115-云下载插件) · [原始需求](https://github.com/xiaoyueRX/Ani-Go/issues/1)

## 2. 编写并导入声明式面板

1. 下载示例，在导入前把 `url` 换成你部署的公开 HTTPS 地址，填写插件名称、作者与版本。示例的 `example.com` 是文档占位地址。
2. 在 **设置 → 插件与自动化** 的添加插件窗口选择 JSON 导入，粘贴或选择文件并确认。也可使用 Ani-Go 登录 JWT，将单份清单作为 JSON 请求体 `POST /api/plugins/save`。
3. 在已启用的插件卡片中配置服务凭据，再点击动作按钮。`events: []` 表示这个示例仅接受用户主动操作。

```json
{
  "id": "cloud-tasks-example",
  "name": "Cloud Tasks",
  "description": "Submit a magnet link to your service and list its cloud tasks.",
  "version": "1.0.0",
  "author": "Example Author",
  "type": "webhook",
  "url": "https://example.com/anigo/actions",
  "events": [],
  "panel": {
    "version": 1,
    "credential": { "label": "Service access token", "required": true },
    "actions": [
      {
        "id": "submit",
        "label": "Submit magnet",
        "placement": "search_result",
        "input": { "key": "magnet", "label": "Magnet link", "placeholder": "magnet:?xt=…", "format": "magnet" },
        "result": "message"
      },
      { "id": "list", "label": "Refresh tasks", "result": "tasks" }
    ]
  }
}
```

| 字段 | 作者约定 |
| --- | --- |
| `id`、`version` | 为插件选用稳定且唯一的 ID。新清单不可使用 `.` 或 `..`。新版本沿用 ID，调整 `version` 后重新导入完整清单。 |
| `dependencies` | 可选的宿主服务名称数组。订阅事件的 Webhook 还需要事件总线。仅提供面板的插件按清单列出的依赖启动。 |
| `panel.version` | 面板协议版本。当前支持 1，省略时按 1 解析。不支持的版本会使导入失败，正在使用的配置继续有效。 |
| `panel.credential` | 可省略。声明 `label` 和布尔值 `required`。清单只描述输入框，实际令牌由用户部署后填写。 |
| `panel.actions[]` | 每项声明唯一动作 `id`、按钮 `label` 和 `result`。声明的动作 ID 也是远端用来分发请求的值。 |
| `placement` | 可选。`search_result` 将带磁力输入、返回 `message` 的动作加入搜索结果的「发送到插件」。省略时动作只在插件面板显示。需要支持此字段的 Ani-Go 版本。 |
| `input` | 需要一个文本输入时声明 `key`、`label`，可加 `placeholder`。`format: "magnet"` 验证磁力链接。输入值以单个字符串传输。 |
| `result` | `message` 渲染服务返回的文字，`tasks` 渲染任务列表和分页。社区清单中的标签按作者提供的文本展示。 |

每个面板声明 1 至 16 个动作。动作 ID 和输入 key 使用小写字母开头，后续可含小写字母、数字、下划线、连字符，总长最多 64 个字符。标签最多 128 个字符，占位文字最多 256 个字符。

用户从搜索结果选择一个插件并确认后，网页会一并提交 `"placement":"search_result"` 和 `"expected_url":"<确认时显示的完整 Webhook 地址>"`。内置插件的目标校验值为空字符串。若其他窗口更换了地址或撤回声明，服务端返回 409，用户需重新选择。服务端只向当前清单配置的目标投递。

### 服务端收发什么

用户在示例卡片提交磁力链接时，浏览器向 Ani-Go 的 `POST /api/plugins/cloud-tasks-example/panel/actions/submit` 发送 `{"input":"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"}`。Ani-Go 向清单的 `url` 发送 HTTPS POST，携带 `Content-Type: application/json`；如已配置面板凭据，还会发送 `Authorization: Bearer <该插件的服务令牌>`。如另设 Webhook 共享密钥，请求也会包含 `X-AniGo-Secret`。浏览器与 Ani-Go 之间的登录 JWT 仅用于 Ani-Go 自身鉴权。

```json
{"action":"submit","input":"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567","page":1}
```

服务用 2xx 和 JSON 作答，例如 `{"message":"任务已受理"}`。展示实际受理结果，超时或回执不明时，用户可先查询任务列表再决定是否重试。查询示例中，服务收到的正文如下。

```json
{"action":"list","input":"","page":1}
```

```json
{
  "page": 1,
  "page_count": 1,
  "tasks": [
    {"info_hash":"0123456789abcdef0123456789abcdef01234567","name":"Example","status":"downloading","progress":35}
  ]
}
```

动作响应必须与声明字段匹配。单条消息最多 2,048 个字符，整份响应至多 1 MiB。每页最多 200 个任务，页码从 1 开始。`tasks` 使用数组，空列表写作 `[]`。状态为 `pending`、`downloading`、`completed`、`failed` 或 `unknown`，进度为 0 至 100 的整数。云端服务保存其任务与文件。本地订阅、下载器和媒体整理继续独立运行。

### 来源、凭据与升级

- 导入前核对清单的作者、动作和完整目标 URL。服务可读取收到的用户输入和 Bearer 凭据，只应配置为该服务签发、具有所需权限的令牌。面板固定调用清单的外部 HTTPS 地址并拒绝重定向。动作请求只接受声明的输入和任务页码。
- `GET /api/plugins/{id}/panel/credential` 用于查看配置状态。`PUT` 接受 `{"access_token":"<服务令牌>","expected_url":"<确认时显示的完整地址或空字符串>"}`（社区目标为当前清单完整地址，内置插件为空字符串；若其他会话已更换目标地址则返回 409），`DELETE` 清除凭据。读取和导出清单时不包含该令牌。插件的事件 Webhook 共享密钥是另一项配置。
- 更新时复核同一 ID 的 URL 与动作。凭据与插件 ID、完整 URL 绑定。更换地址、移除凭据声明或删除插件时，旧面板凭据会从 Ani-Go 清除。移除插件后如需在远端撤销已签发的令牌，请在服务提供方操作。旧的纯事件 Webhook 可继续只声明 `events`。
- 项目随二进制分发内置实现，社区作者可以分发 JSON 文件或其源码链接，由管理员手动导入。Ani-Go 校验清单并渲染其中声明的控件。

## 3. 事件 Webhook

在 `events` 中选择 `download.completed`、`file.organized` 等事件。Ani-Go 向 `url` 发送 `POST`，正文包含配置中的事件名、Unix 秒时间戳和事件自身的 `payload`。这是下载完成事件的精简示例。

```json
{"event":"download.completed","timestamp":1750000000,"payload":{"episode_id":7,"anime_title":"Example","episode":1,"hash":"0123456789abcdef0123456789abcdef01234567","size":1024}}
```

请求头含 `Content-Type: application/json` 和 `User-Agent: Ani-Go-Webhook/<插件ID>`。如果设置了事件 `secret`，共享值原样放入 `X-AniGo-Secret`。接收端可用固定时间比较核对共享值。同类型、同地址的清单更新可省略 `secret` 以沿用原值；显式传入空字符串会清除共享值。面板动作凭据只用于面板请求。需要同时接收事件和动作时，同一个端点按正文中的 `event` 或 `action` 分发。事件由后台投递，单次请求超时 10 秒，生产客户端限制私网目标。[事件协议与投递](../internal/plugin/manager.go) · [事件名称](../internal/core/interfaces.go) · [下载事件字段](../internal/scheduler/scheduler.go)

每个事件订阅按发布顺序逐条执行，可在一个回调执行期间排队 256 条。队满时，当前订阅拒收新投递，其他订阅仍可接收。Go 宿主的 `TryPublish` 返回本次接受和队满的订阅数，以及总线是否关闭。常规 `Publish` 按秒汇总记录拒收数量。一个回调运行超过 5 秒时会记录提示，该订阅在回调实际退出后才处理下一条。[队列与背压](../internal/event/bus.go)

正常关停时，宿主先停止 HTTP 和调度任务，再给事件回调最多 5 秒排空，然后停用插件。超时会写入日志，停用 Webhook 会取消进行中的 HTTP 请求。事件队列保存在内存中，强制退出或断电后无法重放尚未投递的通知。Webhook 接收端应根据业务记录核对遗漏的操作。[关停顺序](../main.go)

## 4. 内置 Go 插件和生命周期

实现 `GetInfo() PluginInfo`、`Init(bus core.EventBus, ctx core.Context) error`、`Stop(bus core.EventBus) error`，并将实例加入 `NewManager` 的内置列表。需要官方归属的项目维护实现还须加入 `officialPluginIDs`。新增内置代码需要重新编译部署。[实际接口](../internal/plugin/types.go) · [注册与加载](../internal/plugin/manager.go) · [内置实例](../internal/plugin/cloud115.go)

管理器将运行期 `PluginContext` 传入 `Init`。可通过 `Subscribe` 订阅总线，通过 `Go` 启动可取消的工作，通过 `RegisterService` 暴露服务，为额外资源调用 `OnDispose`。停用时上下文取消，注销订阅和服务，已登记的清理回调按登记逆序执行。工作协程须自行响应 `ctx.Done()`。整理器的命名钩子是独立的 Go 接口，预览与实际整理都会调用它。过滤与解析拦截器在核心中有类型与独立测试；当前订阅过滤和解析入口尚未接入这两条钩子链。[作用域与清理](../internal/plugin/context.go) · [整理钩子](../internal/core/hooks.go) · [命名预览](../internal/organizer/preview.go)

内置实现可以额外声明 `Dependencies() []core.ServiceType`。管理器在所需服务可用后启动消费者；已启用的插件缺依赖时挂起，服务恢复或替换后重新装配。停用提供者时先停止其消费者。核心服务包括 `database`、`config`、`eventbus`、`source`、`downloader`、`organizer`、`notifier`、`scheduler`、`backup_manager` 和 `task_parser`，元数据提供方可用时另有 `metadata`，AI 客户端可用时另有 `ai`。声明依赖后在 `Init` 通过上下文查找所需服务。宿主可用 `Manager.SetCoreService(name, svc)` 发布或替换服务，以 `nil` 撤销。生命周期管理器保留在宿主中，外部管理通过受鉴权的 API 或 MCP 查询、启停插件。[启动装配](../main.go) · [依赖协调实现](../internal/plugin/dependencies.go)

`GET /api/plugins` 返回配置开关 `enabled`、运行状态 `status` 以及可选错误 `error`。状态定义为 `inactive`、`loading`、`active`、`suspended`、`error`、`disposing`。非空的 `dependencies`、`missing_dependencies` 分别列出所需服务和缺少的服务。例如，初始化失败时配置开关可以开启，同时运行状态为 `error`。[状态定义](../internal/plugin/types.go)

插件的作用域也追踪正在处理的事件回调。停用后若仍有工作在退出，插件保持 `disposing`，待旧实例退出后再按最新开关启动。恢复完整备份会更换社区插件的运行实例，即使清单地址相同，旧实例也不能继续使用恢复后的面板凭据。

有面板的内置插件在自己的作用域用 `SetService(PanelService, runner)` 注册 `PanelRunner`。它应返回 Ani-Go 的 `PanelResult` 和 `PanelTask`，并在适配器中把服务商错误映射为面板错误。需要部署环境令牌时可以在同一作用域设置 `PanelCredentialService`，值为 `PanelCredentialStorage{EnvironmentKey: "服务自己的环境变量名"}`。省略这项时采用插件自己的数据库凭据。环境覆盖生效时凭据在网页端只读。社区清单只声明输入框，实际访问目标由其 URL 决定。[面板类型](../internal/plugin/panel.go) · [凭据契约](../internal/plugin/panel_credentials.go) · [内置接入示例](../internal/plugin/cloud115.go)

如果恢复事务回滚，原实例保留；事务已提交但重载失败时，服务端撤销旧配置的插件实例并返回 503，管理员修复配置后可重载。

## 5. 设计参考

Ani-Go 独立实现 Go 插件运行时、JSON 面板协议及插件分发。按作用域登记服务、统一撤销副作用的设计借鉴了 DeepSeek Harness 文档对 Cordis 的说明。该版本的上游示例展示了如何让服务注册和事件订阅随作用域一起释放。[上游 Cordis 说明](https://github.com/deepseek-ai/deepseek-harness/blob/4b399c39a6d1479cd84ca591b4d7c88e24d38a96/docs/cordis-primer.md#L5-L15) · [Ani-Go 作用域实现](../internal/plugin/context.go)
