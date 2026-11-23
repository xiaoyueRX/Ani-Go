# Connect an MCP client

[简体中文](03_MCP_SPEC.md) · [API reference](API.md)

Ani-Go exposes MCP over HTTP and SSE. Initialization advertises protocol version `2024-11-05` and tool capabilities. A remote SSE client can manage subscriptions, inspect downloads and control plugins. [Protocol implementation](../internal/mcp/server.go)

## Connect

1. Sign in as an administrator and generate an MCP token in Settings. The management endpoint is `POST /api/mcp/token/generate` and requires a login JWT. Generating a new token replaces the previous one. Save the value shown when it is generated.
2. Select remote SSE in the client. Connect to `https://your-host/mcp/sse` at the site root with the header `Authorization: Bearer <MCP token>`. A reverse proxy must forward the SSE stream.
3. The SSE `endpoint` event supplies `/mcp/messages?session_id=…`. The client posts JSON-RPC requests there with the same credential. Responses arrive on the open SSE connection. The server also accepts a `token` query parameter. Sending the header keeps the token out of ordinary URL logs. [Routes and token lookup](../internal/api/server.go)

Call `tools/list` for the exact parameter schemas available in a deployment. The host currently registers the following tools. [Anime operations](../internal/mcp/tools.go) · [Governance tools](../internal/mcp/governance_tools.go)

| Tool | Operation |
| --- | --- |
| `search_anime` | Search releases with `keyword` and optional `source` and `limit`. |
| `list_subscriptions` | List subscriptions and progress with optional `enabled_only`, `limit` and `offset`. |
| `add_subscription` | Create a subscription with `title` and optional `season`, `rss_url` and `subgroup`. The same title can be tracked in different seasons. A duplicate title and season returns a tool error. |
| `remove_subscription` | Remove a subscription by `subscription_id`. |
| `get_downloads` | Inspect the selected downloader, optionally filtering by `status` and paginating. |
| `trigger_refresh` | Start historical supplements for enabled subscriptions in the background. |
| `list_plugins`, `toggle_plugin` | Inspect plugins or change the switch using `plugin_id` and `enabled`. Both tools report runtime state, including missing services when suspended. |
| `get_system_status` | Report version, memory, goroutine count, downloader connectivity and subscription count. |
| `update_system_config` | Write an allowed `key` and `value`. Credentials and internal plugin state have dedicated management endpoints. |

After initialization, this request shows the plugin list and outstanding dependencies.

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_plugins","arguments":{}}}
```
