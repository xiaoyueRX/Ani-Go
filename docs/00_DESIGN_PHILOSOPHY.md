# Ani-Go 设计原则

Ani-Go 面向自托管 NAS。它订阅资源、识别季集、把任务交给用户的下载器，再把完成的媒体整理到媒体服务器可扫描的目录。下载器和媒体服务器各自管理下载与播放。[系统架构](01_ARCHITECTURE.md)

## 功能放在哪里

- 订阅匹配、下载去重和文件整理需要协同维护数据库状态，属于主流程。选择下载器、资源源或元数据提供方时使用 Go 接口。
- 可独立启停的集成使用插件。需要用户手动触发远程动作时可分发 Webhook JSON 面板；需要在进程内订阅事件、提供 Go 服务或调用原生协议时，可编译内置插件。备份调度、REST API 和 MCP 服务由宿主直接装配。[插件作者指南](02_PLUGIN_SPEC.md)
- 支持继续做种的整理方式是硬链接。媒体目录获得独立的目录项，下载器保留原文件路径。跨文件系统时需要复制；关闭硬链接后，整理器采用移动模式。[整理实现](../internal/organizer/organizer.go)

## 插件的生命周期

通过插件上下文登记事件、服务、清理回调及受管协程。后台任务应响应上下文取消，停用或依赖消失时由管理器撤销登记。插件与宿主共处一个 Go 进程；未登记的副作用需要作者自行清理。事件回调或受管协程发生 panic 时会记录日志。初始化失败会反映在插件状态中。[上下文](../internal/plugin/context.go) · [生命周期](../internal/plugin/dependencies.go)

## 资源预算

生产二进制嵌入前端，并采用支持关闭 CGO 的 SQLite 驱动。项目为小型 NAS 设定 15–30 MB 常驻内存和接近空闲的 CPU 目标。评估部署时应记录订阅数、已启用插件、并发量及实际测量值。[构建文件](../Dockerfile)

## Design principles in brief

Ani-Go hands releases to an external downloader and organizes completed files for a media server. Keep subscription matching, deduplication and organization in the core. Use a declarative Webhook panel for a remote service, or a compiled Go plugin for in-process integration. Backup scheduling, HTTP and MCP belong to the host. Plugins must register cleanup and cooperate with cancellation. The memory and idle CPU figures are design targets and depend on the workload. [Architecture](01_ARCHITECTURE_EN.md) · [Plugin authoring](02_PLUGIN_SPEC_EN.md)
