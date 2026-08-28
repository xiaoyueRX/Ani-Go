# Changelog

Ani-Go 的更新日志。所有版本遵循 [语义化版本](https://semver.org/)。

---

## v0.3.0 — 2026-08-29

### 新增

#### 种子自动清理
- **做种自动清理**：下载完成的种子在达到指定做种时间且比率达标后自动从 qBittorrent 删除（仅删种子记录，不删文件）
- **环境变量配置**：
  - `SEED_CLEANUP_ENABLED=true` — 启用/禁用（默认启用）
  - `SEED_CLEANUP_INTERVAL=1h` — 检查间隔（默认 1 小时）
  - `SEED_CLEANUP_MIN_SEED_TIME=48h` — 最小做种时间（默认 48 小时）
  - `SEED_CLEANUP_MIN_RATIO=1.0` — 最小做种比率（默认 1.0）
- **Web UI 配置**：设置页新增「种子自动清理」配置区块，支持开关、间隔、比率设置
- **数据库新增字段**：Episode 模型增加 `Resolution` 字段

#### qBittorrent 标签增强
- **自动标签**：添加种子时自动附带字幕组和分辨率标签
- **标签格式**：`ani-go;字幕组名;1080p`（分号分隔）
- **字幕组来源**：优先使用订阅配置的字幕组（`SubgroupName`），比标题解析更准确
- **分辨率来源**：从种子标题正则解析（支持 4K/2160p/1080p/720p/480p）

### 修复

#### qBittorrent 状态码兼容
- 登录成功：支持 `204 No Content`（qB 标准行为）
- 添加种子：支持 `202 Accepted`（异步处理）、`409 Conflict`（已存在幂等）
- 自动重试：认证失败时自动重新登录并重试（最多 1 次）

#### 前端修复
- 弹窗滚动：编辑弹窗 `overflow-hidden` → `overflow-y-auto max-h-[85vh]`，修复内容超出时无法滚动的问题

### 优化

#### Docker 构建
- 中科大镜像源：Alpine apk 源切换为 `mirrors.ustc.edu.cn`，解决 GFW 环境构建慢/失败
- 基础镜像：`alpine:3.22` → `alpine:3.20`（兼容性更好）

---

## v0.2.0 — 2026-08-28

### 新增
- qBittorrent 下载器深度集成
- Mikan RSS 自动订阅与补全扫描
- 种子自动清理配置（Web UI）

### 修复
- qBittorrent 204/202/409 状态码兼容
- 前端弹窗滚动修复

---

## v1.4.0 — 2026-08-24

### 新增
#### 通知系统全面重构
- **v2 通知器适配器**：Telegram/钉钉/企业微信/飞书/OneBot(QQ) 真实发送逻辑
- **通知管理器**：Worker 池并发处理、指数退避重试、死信队列
- **EventBus 深度集成**：订阅 5 大核心业务事件
- **测试通知 API**：`POST /api/notify/test` 支持单渠道/全渠道测试
- **WebUI 设置页**：notify 标签页各渠道字段新增 🔔 测试按钮

#### 整理器增强
- 硬链接优先（同文件系统），跨设备自动回退复制+删除源文件

---

*完整更新历史请查看 [GitHub Releases](https://github.com/xiaoyueRX/Ani-Go/releases)*
