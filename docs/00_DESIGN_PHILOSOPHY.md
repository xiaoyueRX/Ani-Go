# Ani-Go 架构设计规范与核心准则 (Architecture & Design Guidelines)

> **说明**：本项目严格遵循微内核与插件化解耦设计原则。在构思或实现新功能前，请务必参考本文档的模块判定标准与工程规范。

---

## 一、 系统定位与核心主干

Ani-Go 致力于为用户提供**极简、轻量、稳定、低心智负担的自动化追番与媒体整理体验**。

系统的核心主干严格收敛为以下五步流水线闭环：

```
RSS 订阅抓取 ──► 标题与集数消歧 ──► 下载器调度 (qB/Aria2) ──► 规范重命名与硬链接 ──► 交付 Jellyfin 播放
```

**以上五步构成了 Ani-Go 的微内核（Core）。主干之外的所有功能均应以模块化扩展形式存在。**

---

## 二、 核心功能 vs 附加功能：解耦判定标准 (The Decision Matrix)

在规划与实现新功能时，请根据以下逻辑进行模块归属判定：

```mermaid
flowchart TD
    A[规划新功能] --> B{该功能是否直接参与核心五步流水线?}
    B -- 是: 属于核心主干 --> C[直接写入微内核 Core 模块]
    C --> C1[规范要求: 纯 Go 编译, 零额外运行时, 内存 < 30MB]
    
    B -- 否: 属于辅助/扩展功能 --> D[实现为独立插件 (Plugin)]
    D --> D1[纳入内置或外置插件体系]
    D --> D2[实现完整停用逻辑 Killswitch]
    D --> D3[配置提供自描述 Schema 与配套 Skill 指南]
```

### 1. 核心功能判定范围 (Core)
符合以下标准的逻辑属于微内核核心功能：
- 基础的 RSS / Mikan / Bangumi 订阅源拉取；
- 基础本地正则解析与集数提取（以及基于官方单集放送日历的免费时间戳对齐）；
- 核心下载客户端调度（qBittorrent / Aria2）；
- 媒体文件硬链接与规范路径生成；
- 刷新 Jellyfin/Emby 媒体库的基础 API 调用。

### 2. 附加功能判定范围 (Plugin)
非核心流水线强依赖的功能，**均应作为独立插件实现**：
- **消息推送中心**（无论接入多少种通知渠道，均作为可随时停用的插件管理）；
- **AI 智能助手与大模型消歧**（作为可选增强模块，配备 Token 消耗限额与用户确认开关）；
- **MCP 外部智能体控制服务**（供外部 Claude/Cursor 调用的控制层接口）；
- **本地 NFO 与海报刮削**（作为入库事件的后续处理扩展）；
- **网盘同步 / WebDAV 备份 / 云端冷归档**；
- **历史数据统计与图表呈现**；
- **外部平台（AutoBangumi / Bilibili）数据迁移导入工具**。

---

## 三、 插件开发设计原则 (Plugin Design Principles)

所有扩展插件均需满足以下技术规范：

### 原则 1：彻底休眠与资源释放（Complete Killswitch）
当用户在控制台中停用插件时，插件在后台必须立即释放全部资源：
- 停止运行该插件所属的所有后台 Goroutine 轮询；
- 不再向外发起任何 HTTP 或网络连接请求；
- 停止向数据库日志表写入非必要的运行流水；
- 前端对应的设置界面应进行相应的隐藏或停用提示。

### 原则 2：故障隔离与异常捕获（Fault Isolation）
Go 语言中子协程未捕获的 panic 会导致进程异常退出。
- 插件的事件监听与核心回调函数必须配置 `defer recover()` 异常保护机制；
- 插件自身的异常仅记录为告警日志并自动进入休眠状态，**确保核心下载与整理流水线正常运转**。

### 原则 3：自描述配置 Schema 与 Agent Skill 指南 (Self-Describing & Skill-Driven)
- **前端自描述（Schema）**〔规划中，尚未实现〕：设计目标是由插件以 `ConfigSchema` 声明参数、通用表单组件动态渲染，避免频繁侵入式修改 Vue 路由代码。当前实现中插件元数据仅为 `PluginInfo`（不含配置 Schema），插件参数统一走 settings 键值表；
- **智能体指南（Skill）**〔规划中〕：设计目标是复杂插件附带 `skill.md` 提示词说明，便于通过 MCP 接入的外部智能体（Claude/Cursor）理解其业务场景与调用参数。当前插件文档统一收录于 `docs/02_PLUGIN_SPEC.md`。

### 原则 4：开放性与通用钩子（Open & Escape Hatches）
- 插件通过订阅系统事件总线挂载业务逻辑。真实事件常量定义于 `internal/core/interfaces.go`：`subscription.added`、`subscription.removed`、`download.started`、`download.progress`、`download.completed`、`download.failed`、`file.organized`、`episode.identified`、`anime.matched`、`supplement.triggered`、`supplement.completed`、`task.parsed`；
- 事件负载统一为 `core.Event{ Type string, Payload map[string]interface{}, Time time.Time }`，方便插件存取自定义参数；
- 已落地的通用通道为 **Webhook**（HTTP POST + `X-AniGo-Secret` 签名头）；本地 Shell 脚本通道为规划项，尚未实现。

---

## 四、 低资源运行环境保障 (Low-Resource Environment Guarantee)

- **适用环境**：保障在 J1900、N3450、树莓派 4B 等常见轻量级 NAS 与工控硬件上稳定运行；
- **资源基准**：空载运行内存保持在 **15MB ~ 30MB** 区间，空载 CPU 占用恒为 **0.0%**；
- **分发标准**：单文件静态二进制分发，无外部运行时依赖，避免捆绑笨重的 Python/Node 解释器。

---

## 五、 代码提交前的自检清单 (Pre-Flight Checklist)

在提交新功能代码前，请对照以下准则确认：

- [ ] **功能归属**：该功能是否属于核心五步流水线？若非核心，是否已组织为独立插件？
- [ ] **停用测试**：插件在停用状态下，是否完全停止后台调度与日志输出？
- [ ] **异常保护**：插件相关逻辑是否包含 `defer recover()` 容错兜底？
- [ ] **成本与配额提示**：若涉及外部大模型或计费 API，是否已在界面设置明确的消耗提示与每日调用限额？
