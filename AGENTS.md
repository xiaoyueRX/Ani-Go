# AGENTS.md

此文件为 AI 助手在处理本仓库代码时提供指导。

## 技术栈与架构
- **语言**: Go 1.22+
- **数据库**: SQLite via GORM (`github.com/glebarez/sqlite` - 纯 Go 驱动，无 CGO 依赖)
- **架构**: 接口驱动 (Hexagonal/Clean Architecture)
- **核心接口**: 定义在 `internal/core/interfaces.go` (Source, Downloader, MetadataProvider, Organizer, Notifier, EventBus)

## 常用命令
- **运行**: `go run .` (在项目根目录执行)
- **构建**: `go build ./...`
- **依赖管理**: `go mod tidy`

## 项目特定约定与注意事项
- **数据库驱动**: 必须使用 `github.com/glebarez/sqlite` 而不是 `gorm.io/driver/sqlite`，以避免在 Windows 上产生 CGO 依赖。
- **配置管理**: 通过 `internal/config/config.go` 加载。环境变量的优先级高于默认值。
- **敏感数据**: 绝对不要在代码中硬编码 token 或密码（例如 `MIKAN_RSS_URL`, `QB_PASS`）。始终使用环境变量。
- **文件编码**: 所有文件必须是无 BOM 的 UTF-8 编码。避免使用 PowerShell 默认的 `>` 重定向，因为它可能会破坏编码。
- **路径处理**: 注意 Docker 容器内路径与宿主机路径的映射关系（例如，容器内的 `/TV` 对应宿主机的 `/vol2/1000/TV`）。
- **可扩展性**: 新功能（例如新的下载器、新的资源站）必须实现 `internal/core/interfaces.go` 中对应的接口，而不是直接修改核心逻辑。
- **事件总线 (Event Bus)**: 使用 EventBus 进行组件间的通信（例如，在下载完成后触发文件整理）。
