# Ani-Go 系统架构

[English](01_ARCHITECTURE_EN.md) · [目录索引](DIRECTORY_STRUCTURE.md)

## 从订阅到媒体库

1. 调度器读取已启用的订阅和 RSS 配置。它按订阅源、标题及字幕组等条件筛选资源。历史补全还可按标题检索其他已配置的资源站。[订阅匹配](../internal/scheduler/subscription_feeds.go) · [调度器](../internal/scheduler/scheduler.go)
2. 本地解析器提取季集、特别篇和合集。遇到歧义，调度器在满足条件时按播出日期比对，或者调用可选 AI。蜜柑、Bangumi 与 TMDB 的条目标识分别对应各自的数据源。[季集识别](04_AUTOPARSER_SPEC.md)
3. 调度器记录下载任务，交给选中的 qBittorrent、Transmission 或 Aria2。完成后读取下载器报告的文件路径。合集可能对应多集。[下载队列](../internal/scheduler/queue.go) · [下载器](../internal/downloader)
4. 整理器应用命名模板和已登记的命名钩子，生成 TV、电影和特别篇路径。硬链接模式保留下载源，必要时复制；移动模式会迁移源文件。媒体服务器扫描自己的媒体库目录。[整理器](../internal/organizer/organizer.go) · [整理预览](../internal/organizer/preview.go)

网页单条、批量和 MCP 新建订阅共用同一数据库事务。同名作品可分别追不同季，并发创建同一季只保存一条记录。若启用了同名多季，共享 RSS 与历史补全采用明确季号或已确认的作品来源，单独绑定在某季的 RSS 仍可接受未标季的发布。时间表优先按来源 ID 关联订阅；按标题回退时只关联唯一匹配的订阅。[订阅创建](../internal/database/subscription.go) · [时间表关联](../internal/api/schedule_subscriptions.go)

修改轮询周期后，调度器会在设置提交时重新设置对应计时器。同一订阅的手工和定时补全会共享正在执行的历史查询。进度按订阅季已整理的正片集号去重。已知总集数时只计入该范围内的集号。使用 TMDB 补全订阅时，集数取自所选季。退出时会取消并等待已接纳的工作，再释放插件。[后台调度](../internal/scheduler/runtime.go) · [剧集计数](../internal/database/progress.go)

## 运行时与控制接口

宿主装配数据源、元数据、下载器、SQLite、调度器和插件管理器。插件可订阅整理事件，或声明所需服务。启用开关表示用户意图，运行状态反映插件能否工作。缺少依赖的插件进入 `suspended`，服务恢复后可重新激活。[启动装配](../main.go) · [插件规范](02_PLUGIN_SPEC.md)

Web 静态资源嵌入 Go 二进制。管理界面通过 REST API 操作宿主；MCP 客户端通过 SSE 连接并调用服务器注册的工具。事件总线在进程内异步广播，命名拦截器同步参与整理。备份和数据恢复通过单独的宿主服务处理。[HTTP 路由](../internal/api/server.go) · [MCP](03_MCP_SPEC.md) · [备份](../internal/backup)
