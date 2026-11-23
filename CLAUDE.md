# Claude Code 项目入口

仓库的执行和开发约定见 [AGENTS.md](AGENTS.md)。使用项目自己的服务注册与插件协议；设计扩展先核对 Go 接口及作用域，再检查现有官方插件和社区面板能否覆盖需求。[插件开发规范](docs/02_PLUGIN_SPEC.md)

## 当前项目状态

当前功能、配置和入口以 [项目说明](README.md)、[开发计划](docs/DEVELOPMENT_PLAN.md) 和 [API 手册](docs/API.md) 为准。改动后维护受影响说明及其中英文版本。在 Web 生产构建后运行全仓 Go 竞态测试，检查最终二进制中的用户入口。历史版本变化见 [发布记录](CHANGELOG.md)。
