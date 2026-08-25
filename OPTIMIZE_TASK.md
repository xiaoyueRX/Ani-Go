# Ani-Go 性能优化任务书 (2026-08-26)

## 目标
优化 Ani-Go 的 CPU 和内存占用。当前容器内存 38MB 尚可，但调度器/整理器在高频轮询时 CPU 有尖峰。

## 排查与优化点（按优先级）

### 1. 轮询间隔与抖动
- 检查 `internal/scheduler/scheduler.go` 的 ticker 间隔
- 若存在 <30s 的高频轮询（qB 扫描/RSS 轮询），改为 60s+ 并加随机抖动（±20%），避免多定时器同时触发

### 2. DB 查询风暴
- 日志中大量 `record not found` 来自 `scheduler.go:236/239` 的逐条查询
- 改为批量查询：一次拉取所有 downloading 状态的 episodes 到 map，再在内存匹配 qB 任务列表
- GORM 开启 `PrepareStmt: true` 复用预编译语句

### 3. HTTP 客户端复用
- 检查是否每请求新建 http.Client / Transport
- 统一用包级单例 client，MaxIdleConnsPerHost=10, IdleConnTimeout=90s

### 4. 内存
- RSS 轮询解析若用 `ioutil.ReadAll` 大响应，改用 `json.Decoder` 流式
- 定期（每小时）`debug.FreeOSMemory()` 可选，不强制

## 验收标准
- `go build ./...` 通过
- `go test ./...` 通过
- 新增/修改处有注释说明理由
- 完成后输出 OPTIMIZEOK 标记

## 约束
- 不改任何业务逻辑语义
- 不引入新依赖
