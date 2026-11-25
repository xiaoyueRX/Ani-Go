# Ani-Go 开发里程碑与当前工作

[English](DEVELOPMENT_PLAN_EN.md) · [版本发布记录](../CHANGELOG.md)

## 已发布的里程碑

| 阶段 | 交付内容 |
| --- | --- |
| 项目早期至 v0.5.x | 建立 RSS 订阅、历史补全、多个下载器与资源站、媒体整理、Web 管理、数据迁移和通知。v0.5.x 陆续加入下载器切换、通知记录与 RSS 检查工具。 |
| v0.6.0–v0.6.1 | 引入原生 MCP 服务、放送日期辅助消歧；后续扩展通知模板、微信与 Signal 通知及 TMDB 系列归并。 |
| v0.6.2 | 引入 Go 插件作用域、可撤销的资源登记及生命周期状态。命名、过滤和解析的拦截接口在核心定义；实际接入点以当前架构说明为准。 |
| v0.7.0 | 适配 qBittorrent 5.x 的暂停/恢复端点，更新 SSE 处理及多平台发布工作流。 |
| v0.8.0 | 重构 Web UI v2（编辑部放映厅、主题工坊与订阅全链路）、完善自有插件治理与官方 115 云下载、增强合集整理/卷标识保护/可移植备份与自更新，优化性能并收敛发行包至 5 个 64 位现代平台。 |

具体版本、归档和校验文件见 [GitHub Releases](https://github.com/xiaoyueRX/Ani-Go/releases)。

## 本源码树

- **下载与整理**：按订阅读取 RSS，历史补全遵守标题与字幕组过滤；下载记录、合集文件与媒体整理共享季集信息。核对下片和整理时，应同时检查下载器报告的文件与最终媒体路径。[系统架构](01_ARCHITECTURE.md)
- **插件与 MCP**：官方归属、受管服务依赖、挂起/恢复、JSON 声明式面板及官方 115 磁力云下载已随 v0.8.0 交付。插件控制器可返回缺失依赖。部署中可用的功能由所运行的构建决定。[插件作者指南](02_PLUGIN_SPEC.md) · [MCP 工具](03_MCP_SPEC.md)
- **发布验证**：前端回归和构建、Go 竞态测试及编译在 CI 中执行。容器镜像和平台归档有独立发布工作流。[CI](../.github/workflows/ci.yml) · [镜像构建](../.github/workflows/docker-build.yml) · [发行包](../.github/workflows/release.yml)

待处理的功能需求和故障见 [项目 Issues](https://github.com/xiaoyueRX/Ani-Go/issues)。
