# MCP 客户端接入

[English](03_MCP_SPEC_EN.md) · [接口文档](API.md)

Ani-Go 提供使用 HTTP + SSE 的 MCP 服务。初始化握手公布协议版本 `2024-11-05` 和工具能力。支持远程 SSE 的客户端可以查询订阅、控制插件和调用下载相关工具。[协议实现](../internal/mcp/server.go)

## 连接

1. 管理员登录 Web 界面，在设置中生成 MCP 令牌。对应的管理 API 是 `POST /api/mcp/token/generate`，需要登录 JWT。新令牌替换此前实例使用的令牌；请保存生成时返回的值。
2. 在客户端选择远程 SSE，连接 `https://你的主机/mcp/sse`，使用请求头 `Authorization: Bearer <MCP令牌>`。该地址位于站点根路径。反向代理需转发 SSE 事件流。
3. 客户端从 SSE 的 `endpoint` 事件取得 `/mcp/messages?session_id=…`，以相同凭据向该地址 POST JSON-RPC 请求。响应通过已建立的 SSE 连接返回。服务器也支持 `token` 查询参数；请求头可避免令牌进入常规 URL 日志。[路由与令牌读取](../internal/api/server.go)

执行 `tools/list` 可获得部署版本的工具名称、参数和必填项。以下为当前宿主注册的工具。[追番工具](../internal/mcp/tools.go) · [治理工具](../internal/mcp/governance_tools.go)

| 工具 | 用途 |
| --- | --- |
| `search_anime` | 用 `keyword` 搜索资源，可传 `source`、`limit`。 |
| `list_subscriptions` | 查看订阅及进度，可用 `enabled_only`、`limit`、`offset` 筛选与分页。 |
| `add_subscription` | 用 `title` 创建订阅，可传 `season`、`rss_url`、`subgroup`。同标题同季返回工具错误，同标题可分季订阅。 |
| `remove_subscription` | 按 `subscription_id` 取消订阅。 |
| `get_downloads` | 查询当前下载器的队列，可按 `status` 筛选及分页。 |
| `trigger_refresh` | 在后台为启用中的订阅触发历史补全。 |
| `list_plugins`、`toggle_plugin` | 查询插件或用 `plugin_id`、`enabled` 切换配置。返回实际运行状态；挂起时列出缺失服务。 |
| `get_system_status` | 查看版本、运行内存、协程数、下载器连通性和订阅数。 |
| `update_system_config` | 写入允许通过 MCP 管理的 `key` 和 `value`。凭据和内部插件状态使用各自的管理入口。 |

例如，完成初始化后可发送以下请求，查看当前插件及缺失依赖。

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_plugins","arguments":{}}}
```
