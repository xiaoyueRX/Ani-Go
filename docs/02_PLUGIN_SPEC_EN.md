# Ani-Go plugin authoring and architecture specification

[简体中文](02_PLUGIN_SPEC.md) · [Importable JSON example](../examples/plugins/cloud-tasks.json) · [Plugin API](API.md#10-插件管理)

This document describes the current source tree. Check `GET /api/health` for the version running in a deployment. The default build version is defined in `main.go`. Older deployments need a build that includes declarative panels to use the form and action endpoints below. [Build version](../main.go)

## 1. Choose an extension type

| Goal | What to ship | Where the code runs |
| --- | --- | --- |
| Receive download or organization events | A `type: "webhook"` manifest listing its `events` | Ani-Go posts JSON to the author's service |
| Give users a credential field, buttons, and a task list | A Webhook manifest containing `panel` and a public HTTPS endpoint | Ani-Go renders the form, validates input, and calls the author's service |
| Integrate local events, services, or a provider-specific protocol | A Go implementation of `BuiltInPlugin` compiled into Ani-Go | The built-in code runs in the Ani-Go process |

The server assigns `origin`. An `official` plugin is a built-in implementation on the project's official list. Imported Webhooks and other built-ins receive `community`. The `is_builtin` field describes distribution. The `author` and `author_url` fields provide credit. Imported manifests cannot claim a reserved built-in ID or obtain official status through an author name. [Provenance and reserved IDs](../internal/plugin/provenance.go)

For example, `cloud-115` is an official Ani-Go plugin maintained by xiaoyueRX that calls the 115 Open Platform. NaJue originally requested magnet submission. The `cloud-tasks-example` below targets a separate third-party service chosen by the author. That service receives credentials supplied for this example. [Official plugin setup](../README_EN.md#115-cloud-downloads) · [Original request](https://github.com/xiaoyueRX/Ani-Go/issues/1)

## 2. Build and import a declarative panel

1. Download the example. Replace `url` with your own public HTTPS endpoint and set your plugin name, author, and version. The `example.com` URL is a documentation placeholder.
2. In **Settings → Plugins & Automation**, open the add-plugin dialog, choose JSON import, paste the manifest or select its file, and confirm. Alternatively, send one manifest object to `POST /api/plugins/save` using an Ani-Go login JWT.
3. Enter the service credential in the enabled plugin's card, then choose an action. With `events: []` the example runs only when a user acts.

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

| Field | Contract |
| --- | --- |
| `id`, `version` | Choose a stable unique plugin ID. New manifests cannot use `.` or `..`. To update, keep the ID, set the new version, and reimport the complete manifest. |
| `dependencies` | Optional array of required host service names. Webhooks that subscribe to events also require the event bus. Panel-only plugins use the dependencies listed in their manifest. |
| `panel.version` | Panel protocol version. The supported value and default is 1. An unsupported version fails import and preserves the installed configuration. |
| `panel.credential` | Optional. Declare `label` and a boolean `required`. The manifest describes the field. The user enters its value after deployment. |
| `panel.actions[]` | Each action declares a unique `id`, a button `label`, and a `result`. Dispatch requests at the remote service by action ID. |
| `placement` | Optional. Use `search_result` on a magnet input action returning `message` to appear under **Send to plugin** in search results. Omitting it keeps the action in the settings panel. Requires an Ani-Go build that recognizes this field. |
| `input` | For a single text field, provide its `key` and `label` and an optional `placeholder`. Use `format: "magnet"` for magnet-link validation. The value travels as one string. |
| `result` | `message` renders response text. `tasks` renders a paginated task list. Community labels appear as authored. |

A panel declares 1–16 actions. Action IDs and input keys start with a lowercase letter and may contain lowercase letters, digits, underscores, and hyphens, up to 64 characters. Labels allow up to 128 characters and placeholders up to 256.

### Requests and responses at your service

When a user confirms a destination in search results, the browser also sends `"placement":"search_result"` and `"expected_url":"<the full Webhook URL shown at confirmation>"`. For built-in plugins the expected URL is an empty string. If another session changes the target or withdraws the placement, the server responds with 409 and the user must select again. Ani-Go sends the action to the URL stored in its manifest.

For the example's submit button, the browser sends `{"input":"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"}` to Ani-Go at `POST /api/plugins/cloud-tasks-example/panel/actions/submit`. Ani-Go sends an HTTPS POST to the manifest's `url` with `Content-Type: application/json`. If the user configured a panel credential, the header is `Authorization: Bearer <this plugin's service token>`. When a separate Webhook secret is set, the request also carries `X-AniGo-Secret`. The browser’s login JWT authenticates the request to Ani-Go only.

```json
{"action":"submit","input":"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567","page":1}
```

Return a 2xx response with JSON such as `{"message":"Task accepted"}`. Report the outcome observed at the service. After a timeout or an ambiguous submission, the user can check the list before retrying. For a list action the service receives the following body.

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

Return exactly the fields declared for the result type. Messages allow up to 2,048 characters, responses up to 1 MiB, and task pages up to 200 entries. Pages start at 1. Return an array for `tasks`, including `[]` for an empty list. Valid statuses are `pending`, `downloading`, `completed`, `failed`, and `unknown`. Progress is an integer percentage from 0 to 100. The remote service owns its tasks and files. Local subscriptions, the configured download client, and media organization continue independently.

### Provenance, credentials, and upgrades

- Before importing, review the author, the declared actions, and the full destination URL. The receiving service can read user input and the Bearer credential. Supply a token issued for that service with the permissions it needs. Ani-Go pins panel actions to the external HTTPS URL in the manifest and rejects redirects. An action request accepts only declared input and task pagination.
- Read configuration status with `GET /api/plugins/{id}/panel/credential`. Send `{"access_token":"<service token>","expected_url":"<full Webhook URL shown at confirmation or empty string>"}` with `PUT` to set it (supply the current manifest URL for community plugins or `""` for built-in plugins; the server rejects stale targets with 409), or call `DELETE` to clear it. Reading or exporting the manifest omits the token. The shared secret used for event Webhooks is a separate setting.
- Review the destination and actions before reimporting an existing ID. A panel credential is bound to the plugin ID and full URL. Ani-Go clears the old credential when the address changes, the credential declaration is removed, or the plugin is deleted. If you also need to revoke an issued token after removing the plugin, revoke it at the service. Existing event-only Webhooks can continue to declare just their `events`.
- The project distributes built-in implementations with the binary. Community authors distribute JSON files or links to their source for manual import. Ani-Go validates the manifest and renders its declared controls.

## 3. Event Webhooks

List events such as `download.completed` and `file.organized` in `events`. Ani-Go posts JSON to `url` with the configured event name, a Unix timestamp in seconds, and the event-specific `payload`. This abbreviated example shows a download completion.

```json
{"event":"download.completed","timestamp":1750000000,"payload":{"episode_id":7,"anime_title":"Example","episode":1,"hash":"0123456789abcdef0123456789abcdef01234567","size":1024}}
```

Requests carry `Content-Type: application/json` and `User-Agent: Ani-Go-Webhook/<plugin-id>`. When an event `secret` is set, Ani-Go sends its value directly in `X-AniGo-Secret`. Receivers can compare the shared value in constant time. When updating the same type and URL, omit `secret` to keep the shared value, or send an empty string to clear it. Panel credentials apply only to panel actions. If a single endpoint accepts both kinds of traffic, dispatch on `event` or `action` in the body. Delivery runs in the background with a 10-second request timeout. Production clients restrict private network destinations. [Event delivery](../internal/plugin/manager.go) · [Event names](../internal/core/interfaces.go) · [Download event fields](../internal/scheduler/scheduler.go)

Each event subscription processes deliveries in publish order, with one callback running and room for 256 more events. A full queue refuses new deliveries for that subscription. Other subscriptions can still accept the event. The Go host's `TryPublish` reports how many subscriptions accepted the event, how many queues were full, and whether the bus was closed. The standard `Publish` logs aggregate refusal counts once per second. A callback that runs longer than five seconds generates a log entry. Its subscription waits for it to return before processing the next event. [Queue and backpressure](../internal/event/bus.go)

During graceful shutdown, the host stops HTTP and scheduler work, then gives event callbacks up to five seconds to drain before stopping plugins. A timeout appears in the log. Stopping a Webhook cancels its in-flight HTTP request. The queue lives in memory, so forced termination or power loss can leave notifications undelivered. A receiver that needs to recover missed work can reconcile against the underlying records. [Shutdown order](../main.go)

## 4. Built-in Go plugins and lifecycle

Implement `GetInfo() PluginInfo`, `Init(bus core.EventBus, ctx core.Context) error`, and `Stop(bus core.EventBus) error`. Add the plugin to the built-ins in `NewManager`. For official project-maintained implementations, also add the ID to `officialPluginIDs`. Adding built-in code requires rebuilding and deploying Ani-Go. [Go interface](../internal/plugin/types.go) · [Registration and loading](../internal/plugin/manager.go) · [Built-in example](../internal/plugin/cloud115.go)

The manager supplies a runtime `PluginContext` to `Init`. Use `Subscribe` for event subscriptions, `Go` for cancellable workers, `RegisterService` to expose a service, and `OnDispose` for additional cleanup. On disable, the context cancels, drops subscriptions and services, and runs registered cleanup callbacks in reverse order. Workers must observe `ctx.Done()`. The organizer's naming hooks are a separate Go interface used by preview and actual organization. Filter and parse interceptors have core types and standalone tests. The subscription filtering and parsing entry points do not yet invoke those two chains. [Scope and cleanup](../internal/plugin/context.go) · [Organizer hooks](../internal/core/hooks.go) · [Naming preview](../internal/organizer/preview.go)

Built-ins can optionally implement `Dependencies() []core.ServiceType`. The manager starts a consumer when its required services are available, suspends enabled consumers whose services disappear, and rebinds them when a service returns or is replaced. Consumers stop before their provider. Startup publishes `database`, `config`, `eventbus`, `source`, `downloader`, `organizer`, `notifier`, `scheduler`, `backup_manager`, and `task_parser`, plus `metadata` when a metadata provider is available and `ai` when the AI client is available. Look up the declared services through the context in `Init`. The host can publish or replace a service with `Manager.SetCoreService(name, svc)`, or withdraw it with `nil`. The host retains the lifecycle manager. External clients can inspect and toggle plugins through the authenticated API or MCP. [Startup wiring](../main.go) · [Dependency lifecycle implementation](../internal/plugin/dependencies.go)

`GET /api/plugins` reports the configured `enabled` flag, runtime `status`, and an optional `error`. The defined states are `inactive`, `loading`, `active`, `suspended`, `error`, and `disposing`. Nonempty `dependencies` and `missing_dependencies` list required and unavailable services. A plugin can be enabled and have status `error` after initialization fails. [State definitions](../internal/plugin/types.go)

The scope also tracks event callbacks already in progress. When work is still exiting, the plugin stays in `disposing`. After the previous instance exits, the manager applies the latest enablement setting. Restoring a full backup replaces running community instances even when their manifest URL is unchanged. Old instances cannot use restored credentials.

A built-in panel registers its `PanelRunner` in its own scope with `SetService(PanelService, runner)`. Return Ani-Go's `PanelResult` and `PanelTask`, and map provider errors to panel errors within the adapter. To support a deployment token, set `PanelCredentialService` in the same scope to `PanelCredentialStorage{EnvironmentKey: "YOUR_SERVICE_TOKEN"}`. Otherwise the panel uses its own stored credential. A token supplied by the environment is read-only in the UI. Community manifests declare the input field and the URL of the receiving service. [Panel types](../internal/plugin/panel.go) · [Credential contract](../internal/plugin/panel_credentials.go) · [Built-in example](../internal/plugin/cloud115.go)

If a restore transaction rolls back, existing instances keep running. If it commits but the configuration cannot be reloaded, the server revokes instances from the old configuration and returns 503. An administrator can fix the stored configuration and reload.

## 5. Design reference

Ani-Go owns its Go runtime, JSON panel protocol and plugin distribution. The design of scoped registrations and reversible cleanup draws on Cordis as described in DeepSeek Harness. The versioned upstream primer illustrates how service registration and event subscriptions can be disposed together. [Upstream Cordis primer](https://github.com/deepseek-ai/deepseek-harness/blob/4b399c39a6d1479cd84ca591b4d7c88e24d38a96/docs/cordis-primer.md#L5-L15) · [Ani-Go scope implementation](../internal/plugin/context.go)
