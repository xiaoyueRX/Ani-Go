# Ani-Go 微内核架构白皮书 (Microkernel Architecture)

## 1. 设计哲学与核心原则

Ani-Go 是专为动漫追番与自动化 NAS 媒体整理而设计的轻量化工具。其架构设计遵循以下核心原则：

1. **微内核极简设计 (Microkernel Core)**：
   主干链路专注实现核心闭环：**「订阅源抓取 ──► 标题与集数消歧 ──► 下载器任务派发 ──► 媒体库规范硬链接 ──► 触发播放器即开即播」**。
   主干采用纯 Go 编写，编译为零外部运行时依赖的单文件二进制。
   常驻运行内存控制在 **15MB ~ 30MB**，空载 CPU 占用保持在 **0.0%**，即使在低功耗硬件（如 J1900、N3450、树莓派等轻量设备）上也能稳定运行。

2. **插件化解耦与资源释放机制 (Pluggable & Zero-Overhead Killswitch)**：
   核心流水线之外的功能（包括多渠道消息推送、本地 NFO 刮削、数据迁移、备份归档以及外部智能体接入）均作为独立插件实现。
   **停用原则**：插件禁用时，系统立即销毁后台协程、注销事件监听、注销 MCP 工具，不发起网络请求，不产生冗余日志，及时释放内存。

---

## 2. 核心主干全流程数据流图

```mermaid
flowchart TD
    subgraph Scheduler ["定时调度中心 (Cron & Interval)"]
        A[Tick 触发] --> B{是否在免打扰或暂停?}
        B -- 否 --> C[读取活跃番剧订阅列表]
    end

    subgraph Pipeline ["四级微内核管道 (Four-Stage Core Pipeline)"]
        C --> D[第一级: 订阅源抓取 (Mikan / Bangumi / ACG.RIP)]
        D --> E[第二级: 三级消歧解析引擎 (Regex + AirDate + AI)]
        E --> F[第三级: 下载器调度派发 (qBittorrent / Aria2)]
        F --> G[第四级: 规范硬链接与媒体库归档 (TV / Movie / SP)]
    end

    subgraph NotifyPlay ["交付与通知 (Delivery)"]
        G --> H[刷新媒体库 (Jellyfin / Emby API)]
        G -. 触发事件 .-> I[事件总线 (EventBus)]
        I -.-> J[可选插件: 消息推送 / NFO刮削 / 自定义脚本]
    end
```

---

## 3. 核心分层架构

### 3.1 数据接入层 (Data Sources)
- 负责对接外部 RSS 或 API 数据源（Mikan 蜜柑计划、Bangumi.tv、AnimeTosho 等）；
- 内置镜像源回退（GFW 穿透）与连接池复用机制；
- 抓取结果标准化为统一的 `TorrentItem` 结构体。

### 3.2 解析与消歧层 (Parser & Disambiguation)
- **第一级（本地正则）**：0ms 极速排除压制、分辨率、年份标签，提取基础季集；
- **第二级（时间戳对齐）**：遇到模糊集数时，自动将种子发布时间（`pubDate`）与 Bangumi 官方单集放送日历对齐，无需消耗 Token 即可解析绝大多数歧义；
- **第三级（AI 上下文兜底）**：针对极少数特殊非标准标题，在具备调用限额和持久化缓存保护的前提下调用大模型辅助解析。

### 3.3 下载调度层 (Downloader Dispatcher)
- 抽象统一 `Downloader` 接口，默认原生驱动 qBittorrent Web API 与 Aria2 RPC；
- 支持做种配额控制（分享率上限/做种时间上限回收任务，严禁删除媒体库文件）；
- 支持死种检测（24小时 0 Peers 疑似死种标记）。

### 3.4 媒体归档层 (Media Organizer)
- **单卷硬链接优先 (Zero IO)**：源文件与目标文件共享物理数据块，秒级整理且完全不占用双倍硬盘空间；
- **源数据保护机制**：整理过程提供前置预览（Dry-run），重整时仅清理并重建目标硬链接，绝不修改、移动或删除源下载目录中的任何原始文件。

---

## 4. 健壮性与隔离保障规范 (Robustness & Safety)

### 4.1 Goroutine Panic 故障隔离
在 Go 语言中，未捕获的 goroutine panic 会导致整个进程崩溃。Ani-Go 在调用所有外部模块、插件以及用户脚本时，**统一采用 `defer recover()` 保护**：
```go
func SafeExecute(name string, fn func() error) (err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("module [%s] panic recovered: %v\nstack: %s", name, r, debug.Stack())
            log.Printf("🚨 [CRITICAL ERROR] %v", err)
        }
    }()
    return fn()
}
```

### 4.2 网络超时与熔断保护 (Circuit Breaker)
所有对外网络请求设置严格的 Context 超时时间（默认 10~15 秒）。当某个外部推送渠道或 RSS 站点连续失败达到阈值（如 3 次），自动进入冷却熔断状态，绝不允许高频死循环重试刷爆磁盘日志。
