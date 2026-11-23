# Codex 开发入口

本仓库的执行约定、分层约束、验证命令与 GitNexus 工作流程见 [AGENTS.md](AGENTS.md)。开发、打包与测试所需的 Node、Go 版本和命令在该文件统一维护。

| 工作内容 | 源码入口 | 配套资料 |
| --- | --- | --- |
| 订阅、下载与整理 | [调度器](internal/scheduler)、[下载器](internal/downloader)、[整理器](internal/organizer) | [项目架构](docs/01_ARCHITECTURE.md) |
| 内置插件与社区扩展 | [插件管理器](internal/plugin)、[通用面板](web/src/components/PluginPanel.vue) | [插件协议](docs/02_PLUGIN_SPEC.md) |
| 网页与 HTTP 接口 | [Vue 前端](web/src)、[HTTP 服务](internal/api) | [前端指南](web/README.md) · [API 手册](docs/API.md) |
| MCP 工具 | [MCP 服务](internal/mcp) | [MCP 规范](docs/03_MCP_SPEC.md) |
| 部署与版本变更 | [容器编排](docker-compose.yml)、[工作流](.github/workflows) | [部署指南](docs/05_DEPLOYMENT.md) · [变更日志](CHANGELOG.md) |

先根据用户入口定位实现，再核对数据存储、运行状态和界面反馈。插件使用 Ani-Go 自有协议；声明、动作、状态和凭据的行为以正常导入、启停、重载及进程退出路径的实测为准。源码注释和说明使用简体中文，更新已有双语文档。
