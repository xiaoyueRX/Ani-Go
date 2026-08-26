# Ani-Go 代理修复任务书 (2026-08-26)

## 问题
容器已注入 HTTP_PROXY/HTTPS_PROXY 环境变量（指向 105 的 mihomo 192.168.1.100:7890），
internal/httpx/httpx.go 的共享 Transport 已配置 http.ProxyFromEnvironment，
但 internal/source/mikan.go 等模块仍使用裸 `&http.Client{Timeout: ...}`（未指定 Transport），
导致代理完全不生效，Mikan 全部镜像探测失败。

## 任务
1. `grep -rn "http.Client{" internal/ cmd/ --include="*.go" | grep -v _test` 找出所有裸建 Client 的位置
2. 统一改为 `httpx.New(timeout)`（保留各自 timeout 语义），或对需要默认 30s 的直接用 `httpx.Default`
   - 注意：mikan.go 第 142 行是结构体字段 httpClient，151 行初始化，220 行临时 Client
   - yucwiki.go、bgmtv、tmdb、通知器等其他文件同样排查
3. 不改变任何超时语义和业务逻辑
4. 完成后运行：
   - `go build ./...`
   - `go test ./...`
5. 全部通过后输出 PROXYOK

## 验收
- grep 确认 internal/source 下不再有裸 `&http.Client{`（_test 文件除外）
- build/test 通过
