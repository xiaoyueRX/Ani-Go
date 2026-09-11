# Ani-Go MCP 协议规范与智能体接入指南 (Model Context Protocol)

## 1. 为什么采用 MCP？

Model Context Protocol (MCP) 是跨大模型与外部系统交互的开放协议标准。

Ani-Go 本身不内置沉重的大语言模型运行时。**通过将 Ani-Go 作为标准的 MCP Server，Ani-Go 专注于数据采集与任务调度，外部智能体负责意图理解与任务决策**：
- 用户使用日常习惯的 **Claude Desktop、Cursor、Dify、Open WebUI** 或移动端 Agent；
- Ani-Go 通过极轻量的纯 Go 协议层暴露标准 **Tools** 与 **Resources**；
- 用户只需发送自然语言，AI 即可理解意图并通过标准接口调用 Ani-Go 完成追番与运维任务。

---

## 2. 通信协议与安全性 (SSE over HTTP)

考虑到 Ani-Go 绝大部分部署在 NAS（fnOS、群晖、Unraid）或云主机 Docker 容器中，Ani-Go 原生支持 **Server-Sent Events (SSE)** 传输层：

- **SSE 握手端点**：`GET /mcp/sse`
- **消息发送端点**：`POST /mcp/messages?session_id=xxx`
- **安全鉴权**：强制要求请求头包含 `Authorization: Bearer <ANI_GO_MCP_TOKEN>`。未携带有效 Token 的连接将被立即拒绝（401 Unauthorized），杜绝公网扫描风险。

---

## 3. 标准工具集清单 (Core MCP Tools)

Ani-Go 内置两组核心工具，并支持已启用插件动态注入扩展工具。

### 3.1 核心追番与业务工具 (Anime Ops)

| 工具名称 | 描述 | 主要参数 |
|---------|------|---------|
| `search_anime` | 全网聚合搜索番剧资源 (Mikan, Bangumi, ACG.RIP) | `keyword` (string), `source` (可选) |
| `list_subscriptions` | 查询当前所有追番订阅状态及各季度集数更新进度 | `enabled_only` (boolean) |
| `add_subscription` | 新增一个番剧追番订阅任务，支持自动配置匹配规则 | `title` (string), `rss_url` (可选), `season` (int), `subgroup` (可选) |
| `remove_subscription` | 取消订阅指定番剧，支持指定是否保留媒体库已有文件 | `subscription_id` (int), `keep_files` (boolean) |
| `get_downloads` | 实时查看 qBittorrent/Aria2 的下载队列、速度与进度 | `status` (all / downloading / completed) |
| `trigger_refresh` | 立即触发一次全系统 RSS 检查与历史剧集自动补全扫描 | 无 |

### 3.2 系统治理与插件控制工具 (System & Plugin Governance)

支持通过 AI 智能体对系统进行配置与插件管理：

| 工具名称 | 描述 | 主要参数 |
|---------|------|---------|
| `list_plugins` | 查看当前系统所有已注册插件、运行状态及其配置 Schema | 无 |
| `toggle_plugin` | 远程启用或停用指定插件（及时生效与释放资源） | `plugin_id` (string), `enabled` (boolean) |
| `get_plugin_config` | 获取指定插件的全部当前配置参数值 | `plugin_id` (string) |
| `set_plugin_config` | 修改插件的指定配置参数并触发热重载 | `plugin_id` (string), `config` (object) |
| `get_system_status` | 获取系统健康指标（存储余量、下载器连通性、版本信息） | 无 |
| `update_system_config` | 修改微内核主干设置（如限速、代理端口、媒体库路径） | `key` (string), `value` (string) |

---

## 4. 客户端配置指南 (Client Configuration)

### 4.1 Claude Desktop 配置

打开 Claude Desktop 配置文件：
- **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

添加以下配置（支持局域网或内网穿透地址）：

```json
{
  "mcpServers": {
    "anigo": {
      "url": "http://192.168.1.100:20001/mcp/sse",
      "headers": {
        "Authorization": "Bearer YOUR_ANI_GO_MCP_TOKEN"
      }
    }
  }
}
```

### 4.2 Cursor / IDE 配置

在 Cursor 的 `Settings -> Features -> MCP` 中点击「Add New MCP Server」：
- **Name**: `anigo`
- **Type**: `sse`
- **URL**: `http://127.0.0.1:20001/mcp/sse`
- **Headers**: `Authorization: Bearer YOUR_ANI_GO_MCP_TOKEN`

---

## 5. Ani-Go Agent Skill (智能体提示词模版)

在与大模型对话前，可将以下内容作为 Claude Project Instructions 或 Agent System Prompt，便于 AI 理解领域业务规则：

```markdown
你是 Ani-Go 番剧管理系统的 AI 助手。你拥有通过 MCP 控制 Ani-Go 的完整能力。

在执行用户追番与运维指令时，请遵循以下领域规则：
1. 分季识别常识：日本动漫续作经常使用绝对集数（例如《咒术回战》第 25 集实为第二季第 1 集）。在添加订阅或重命名时，请调用工具确认季度对应关系。
2. 字幕组优先级：优先推荐简日双语、内嵌或内封 ASS 字幕的高清压制组（如 LoliHouse、VCB-Studio、喵萌奶茶屋）。
3. 谨慎操作原则：在执行删除订阅或清理下载任务前，必须主动向用户确认是否需要保留本地已整理的视频文件。
4. 插件调度：如果用户抱怨通知频繁，请使用 toggle_plugin 关停 notifier 插件；如果用户需要更新通知凭据，使用 set_plugin_config 精确修改。
```
