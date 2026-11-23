# Ani-Go 仓库开发约定

[English](AGENTS_EN.md)

## 架构与构建

Ani-Go 在 NAS 上抓取 RSS，识别季集，交给下载器，完成后整理到媒体库。开启硬链接时优先建链接，失败可回退为复制并保留源文件；关闭硬链接时移动文件。扩展优先通过独立插件实现。服务依赖、启停和资源清理由 Ani-Go 自己的 Go 管理器负责；社区作者使用 Ani-Go JSON 清单声明 Webhook 与面板。官方来源由服务端清单认定。[架构](docs/01_ARCHITECTURE.md) · [插件协议](docs/02_PLUGIN_SPEC.md)

使用 Go 1.25+、标准库 HTTP 路由、GORM 和纯 Go SQLite 驱动 `github.com/glebarez/sqlite`。生产构建必须支持 `CGO_ENABLED=0`。Web 使用 Node 24、Vue 3、TypeScript、Vite、Tailwind CSS、DaisyUI 和 Lucide。先生成 Web 资源再编译 Go，嵌入的界面才与源码一致。

```sh
(cd web && npm ci && npm test && npm run build)
go test -race ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o ani-go .
```

## 开发约定

- 用简体中文沟通、编写 Go 注释和提交说明。文件使用 UTF-8 无 BOM，有中英版本的说明同步维护。变更接口时同时核对手册、示例和 OpenAPI。
- 核心服务通过 `internal/core` 接口访问。内置插件通过作用域登记监听、服务和后台工作；停用后检查取消与释放。外部事件、动作和凭据契约以插件作者规范为准。
- 配置从默认值和部署环境初始化，再合并数据库中的运行设置。各字段可否在线修改及其优先级以 `internal/config/config.go` 和专用处理器为准。密码、令牌使用部署环境或对应配置入口；禁止硬编码、写入普通日志或清单导出。
- 首次启动创建管理员，JWT 密钥持久化，密码使用 bcrypt。公开端点、MCP 专用鉴权及 SSE 接口见 API 手册。
- 更新带有 GORM 默认值的布尔字段时显式写入 `false`。处理软删除时一并核对关联记录、回收及恢复行为。
- Linux 或 WSL 可运行后端测试，跨平台改动须核对目标系统。按实际网络环境配置 Go 代理及镜像。Web 使用 Lucide 图标；开发代理、测试和构建见前端指南。
- NAS 空载内存 15–30 MB、CPU 0.0% 是设计目标。发布性能数字时注明目标机器与实测条件。发布时同步已有变更日志、版本来源与构建产物。
- CI 对主分支的提交和拉取请求构建前端并运行 Go 竞态测试；镜像发布与版本发布遵循各自工作流配置。

## 资料入口

[安装与使用](README.md) · [设计目标](docs/00_DESIGN_PHILOSOPHY.md) · [目录](docs/DIRECTORY_STRUCTURE.md) · [HTTP API](docs/API.md) · [MCP](docs/03_MCP_SPEC.md) · [前端](web/README.md) · [开发计划](docs/DEVELOPMENT_PLAN.md) · [发布记录](CHANGELOG.md)

## 代码影响检查

首次建立或刷新本地索引时，从仓库根目录运行以下命令。索引保存在 `.gitnexus/`，生成的索引仅在本地使用。

```sh
npx gitnexus analyze --skip-agents-md --skip-skills
```

修改函数、类型或方法之前，使用 GitNexus 的 `impact` 查询上游调用者和受影响的流程。若返回 HIGH 或 CRITICAL，先说明涉及哪些入口，并覆盖相应回归。对于 Vue 模板、事件注册等动态调用，同时查找调用点。跨模块重命名使用 GitNexus 的符号重构工具。提交前检查暂存改动：

```sh
npx gitnexus detect-changes --scope staged
git diff --cached --check
```

[调用关系与风险检查](.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md) · [代码图命令](.claude/skills/gitnexus/gitnexus-cli/SKILL.md)
