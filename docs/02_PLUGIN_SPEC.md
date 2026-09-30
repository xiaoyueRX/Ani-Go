# Ani-Go 插件开发规范与自描述指南 (Plugin Specification)

> **文档状态**：本文档描述**当前代码已实现**的插件机制，与 `internal/plugin/` 源码逐项对齐。
> 未实现的能力统一收敛在第 6 节，请勿按该节内容编写生产插件。

## 1. 插件设计核心准则

在 Ani-Go 中，插件系统的唯一目的不是为了堆砌臃肿功能，而是**为了通过彻底隔离与电闸机制，保护微内核主干的纯粹与轻量**。

### 插件设计要求：
1. **资源彻底释放（Resource Cleanup & Killswitch）**：当用户在设置中将插件切换为「禁用」时，插件必须主动解除全部事件订阅（`bus.Unsubscribe`），停止后台协程与轮询。系统表现与该插件未启用状态完全一致。
2. **异常故障隔离（Fault Tolerance）**：插件生命周期与回调均由插件管理器以 `defer recover()` 包裹，插件内部 panic 只记录告警日志，不会导致主进程退出。
3. **状态持久化（Persistent State）**：插件启用状态与自定义插件清单持久化于 SQLite 的 `settings` 表，重启后自动恢复，无需重新配置。

---

## 2. 插件标准接口规范 (Go Interface)

所有内置官方插件均实现 `internal/plugin/types.go` 中的标准接口：

```go
package plugin

import "github.com/xiaoyueRX/Ani-Go/internal/core"

// PluginInfo 插件元数据，同时也是前端插件管理面板的数据契约
type PluginInfo struct {
    ID          string   `json:"id"`           // 插件全局唯一标识，如 "nfo_scraper"
    Name        string   `json:"name"`         // 插件展示名称
    Description string   `json:"description"`  // 插件功能简介
    Version     string   `json:"version"`      // 语义化版本号
    Author      string   `json:"author"`       // 作者信息
    AuthorURL   string   `json:"author_url"`   // 作者主页
    Icon        string   `json:"icon"`         // Lucide 图标名称或 URL
    Enabled     bool     `json:"enabled"`      // 当前启用状态（由管理器回填）
    IsBuiltIn   bool     `json:"is_builtin"`   // 是否内置插件
    Type        string   `json:"type"`         // "builtin" / "webhook" / "script"
    URL         string   `json:"url,omitempty"`    // 自定义插件的回调地址
    Secret      string   `json:"secret,omitempty"` // 自定义插件的签名密钥
    Events      []string `json:"events"`       // 关心的事件类型列表
}

// BuiltInPlugin 内建插件生命周期接口
type BuiltInPlugin interface {
    GetInfo() PluginInfo                            // 返回插件元数据
    Init(bus core.EventBus, ctx core.Context) error // 启用：订阅事件、启动协程
    Stop(bus core.EventBus) error                   // 停用：解除订阅、释放全部资源
}
```

**约定**：
- `Init` 必须对同一插件可重复调用（管理器每次 `Load()` 都会先统一退订再重新启用），实现需自行保证幂等；
- `Stop` 必须在插件停用时被调用，并完成 `bus.Unsubscribe(evType, subID)` 退订，避免事件句柄叠加；
- 事件类型常量统一定义在 `internal/core/interfaces.go`（`EventDownloadCompleted`、`EventFileOrganized` 等 12 个）。

---

## 3. 插件注册、启停与持久化

插件的一切生命周期由 `internal/plugin/manager.go` 的 `Manager` 统一驱动，插件自身不直接操作数据库。

| 机制 | 实现位置 | 说明 |
|------|---------|------|
| 内置插件注册 | `NewManager()` | 在代码中静态注册，当前共 6 个内置插件 |
| 启用状态持久化 | `settings` 表，键名 `plugin_status` | JSON 对象 `{"插件ID": true/false}` |
| 自定义插件清单 | `settings` 表，键名 `custom_plugins` | `PluginInfo` 数组的 JSON 序列化 |
| 默认停用策略 | `isDefaultDisabled()` | `yuc_schedule`、`nfo_scraper` 默认关闭，其余默认开启 |
| 热重载 | `Manager.Reload()` → `Load()` | 先统一退订旧事件，再按最新状态重建订阅 |

`Manager` 对外提供的方法：

```go
func NewManager(bus core.EventBus) *Manager
func (m *Manager) Load()                               // 读取 DB 状态并重建全部插件订阅
func (m *Manager) GetPluginList() []PluginInfo         // 内置 + 自定义插件总览
func (m *Manager) IsPluginEnabled(id string) bool      // 供核心流水线查询插件开关
func (m *Manager) TogglePlugin(id string, enabled bool) error
func (m *Manager) Enable(id string) error
func (m *Manager) Disable(id string) error
func (m *Manager) AddOrUpdatePlugin(p PluginInfo) error // 新增/更新自定义插件
func (m *Manager) DeletePlugin(id string) error
func (m *Manager) Reload()
```

对应的 HTTP 端点（均需 JWT 鉴权）：

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/plugins` | 插件总览（含启用状态与事件列表） |
| `POST` | `/api/plugins/toggle` | 启用/停用指定插件 |
| `POST` | `/api/plugins/save` | 新增或更新自定义插件 |
| `DELETE` | `/api/plugins/{id}` | 删除自定义插件 |
| `POST` | `/api/plugins/reload` | 重载插件配置 |

---

## 4. 内置插件清单

| 插件 ID | 名称 | 关注事件 | 默认状态 |
|---------|------|---------|---------|
| `metadata-mapping` | 元数据映射器 | `subscription.added`、`supplement.triggered` | 启用 |
| `standard-naming` | 媒体库规范重命名 | `file.organized` | 启用 |
| `extended-notifiers` | 消息通知推送（18 个渠道） | 下载与补全相关事件 | 启用 |
| `data-migrate` | 数据导入与迁移 | `data.migrate` | 启用 |
| `yuc_schedule` | 長門番堂 (yuc.wiki) 时间表 | 无（纯数据源扩展） | **停用** |
| `nfo_scraper` | Bangumi NFO 刮削器 | `file.organized` | **停用** |

---

## 5. 自定义插件（Webhook）

当前已实现的自定义插件类型为 **Webhook**：用户在前端登记 `URL`、`Secret` 与关心的事件列表后，管理器会为每个事件建立订阅，事件触发时以 HTTP POST 推送：

```json
{
  "event": "download.completed",
  "timestamp": 1750000000,
  "payload": { "...": "事件原始负载" }
}
```

请求头约定：

| 请求头 | 值 |
|--------|-----|
| `Content-Type` | `application/json` |
| `User-Agent` | `Ani-Go-Webhook/<插件ID>` |
| `X-AniGo-Secret` | 登记插件时填写的 `secret`（未填写则不带此头） |

推送使用 `internal/httpx` 统一客户端，**超时 10 秒**，并在独立 goroutine 中发送且带 `defer recover()`，不阻塞主流水线。

> 说明：`PluginInfo.Type` 预留了 `"script"` 取值，但**当前代码库中没有脚本执行器**，填写 `script` 不会产生任何行为。

---

## 6. 尚未实现的能力（规划中）

以下能力在早期设计稿中出现过，但**当前源码中不存在**，编写插件时请勿依赖：

| 能力 | 说明 |
|------|------|
| `ConfigSchema()` 自描述配置 | 插件目前无独立配置 Schema，前端插件面板只渲染 `PluginInfo` 字段；插件参数化走 `settings` 表 |
| `plugin.json` 外置插件包 | 系统不扫描、不解析 `plugin.json`；外部插件以 `custom_plugins` 记录形式登记 |
| `RegisterMCPTools()` 动态挂载 | 插件无法向 MCP 注册私有工具；MCP 工具集当前为固定 10 项（见 `docs/03_MCP_SPEC.md`） |
| `"script"` 类型插件 | 数据模型已预留枚举值，执行器未实现 |
| 第三方插件仓库 / 在线安装 | 未实现 |

如后续实现上述能力，需同步更新本文档与 `docs/03_MCP_SPEC.md`。

---

## 7. 五分钟开发一个内置插件示例

编写一个只在文件整理完成后记录一条审计日志的极简内置插件：

```go
package plugin

import (
    "log"

    "github.com/xiaoyueRX/Ani-Go/internal/core"
)

type AuditPlugin struct {
    subID core.SubscriptionID // 事件订阅句柄，停用时用于精确退订
    bus   core.EventBus
}

// GetInfo 返回插件元数据，管理器与前端插件面板均依赖此结构
func (p *AuditPlugin) GetInfo() PluginInfo {
    return PluginInfo{
        ID:          "audit-logger",
        Name:        "整理审计日志",
        Version:     "1.0.0",
        Description: "在文件整理完成后记录一条审计日志。",
        Author:      "Ani-Go Team",
        Icon:        "FileText",
        Type:        "builtin",
        IsBuiltIn:   true,
        Events:      []string{core.EventFileOrganized},
    }
}

// Init 启用插件：订阅事件（管理器已在外层包裹 defer recover）
func (p *AuditPlugin) Init(bus core.EventBus, ctx core.Context) error {
    p.bus = bus
    p.subID = bus.Subscribe(core.EventFileOrganized, func(ev core.Event) {
        log.Printf("📝 [AUDIT] 剧集已成功归档: %v", ev.Payload)
    })
    return nil
}

// Stop 停用插件：必须解除订阅，保证 0 协程、0 事件句柄残留
func (p *AuditPlugin) Stop(bus core.EventBus) error {
    if p.bus != nil {
        p.bus.Unsubscribe(core.EventFileOrganized, p.subID)
        p.bus = nil
    }
    return nil
}
```

最后在 `internal/plugin/manager.go` 的 `NewManager()` 中追加一行注册：

```go
m.builtInPlugins = append(m.builtInPlugins, &AuditPlugin{})
```

如需默认关闭，再把插件 ID 加入 `isDefaultDisabled()` 的判定列表。
