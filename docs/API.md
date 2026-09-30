# Ani-Go API 文档

> **版本**: v0.6.0  
> **基础路径**: `/api`  
> **认证方式**: JWT Bearer Token / MCP Bearer Token  
> **内容类型**: `application/json; charset=utf-8`

---

## 目录

1. [快速开始](#快速开始)
2. [认证与鉴权](#1-认证与鉴权)
3. [健康检查](#2-健康检查)
4. [用户信息](#3-用户信息)
5. [订阅管理](#4-订阅管理)
6. [剧集管理](#5-剧集管理)
7. [下载队列](#6-下载队列)
8. [设置管理](#7-设置管理)
9. [搜索与发现](#8-搜索与发现)
10. [Mikan 镜像管理](#9-mikan-镜像管理)
11. [插件管理](#10-插件管理)
12. [任务解析](#11-任务解析)
13. [数据迁移](#12-数据迁移)
14. [版本信息](#13-版本信息)
15. [图片代理](#14-图片代理)
16. [MCP 协议端点](#15-mcp-协议端点)
17. [前端 API 调用参考](#16-前端-api-调用参考)
18. [错误码说明](#17-错误码说明)

- [端点总览（全部 69 条路由速查）](#端点总览全部-69-条路由速查)

---

## 端点总览（全部 69 条路由速查）

下表覆盖 `internal/api/server.go` 中注册的**全部路由**。标 📖 的分组在下方有完整字段说明；未标 📖 的为速查索引，字段细节请查阅对应 Handler 源码。

> 除 `GET /mcp/sse` 与 `POST /mcp/messages` 挂载于根路径外，其余端点均以 `/api` 为前缀。

### 📖 认证与用户

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `POST` | `/api/login` | 公开 | 登录获取 JWT Token |
| `GET` | `/api/me` | JWT | 当前用户信息 |
| `POST` | `/api/user/change-password` | JWT | 修改当前用户密码 |
| `POST` | `/api/user/avatar` | JWT | 上传用户头像 |
| `GET` | `/api/user/avatar` | 公开 | 读取用户头像 |

### 📖 健康检查与版本

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/health` | 公开 | 服务健康检查 |
| `GET` | `/api/version` | JWT | 版本号与内置更新日志 |

### 📖 订阅与剧集

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/subscriptions` | JWT | 订阅列表（支持 `enabled` / `completed` 过滤） |
| `POST` | `/api/subscriptions` | JWT | 创建订阅 |
| `POST` | `/api/subscriptions/batch` | JWT | 批量创建订阅（最多 20 部） |
| `GET` | `/api/subgroups` | JWT | 已有字幕组去重列表 |
| `GET` | `/api/subscriptions/{id}` | JWT | 订阅详情 + 剧集列表 |
| `PUT` | `/api/subscriptions/{id}` | JWT | 部分更新订阅 |
| `DELETE` | `/api/subscriptions/{id}` | JWT | 删除订阅（`?delete_files=` 可选） |
| `POST` | `/api/subscriptions/batch-delete` | JWT | 批量删除（最多 100 部） |
| `POST` | `/api/subscriptions/batch-restore` | JWT | 批量恢复软删除订阅 |
| `POST` | `/api/subscriptions/{id}/trigger-supplement` | JWT | 触发单个订阅历史补全 |
| `POST` | `/api/subscriptions/supplement-all` | JWT | 异步触发全部订阅补全（预过滤后立即返回） |
| `PUT` | `/api/episodes/{id}/status` | JWT | 手动切换剧集状态 |

### 📖 下载队列

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/downloads` | JWT | 下载队列总览 |
| `POST` | `/api/downloads` | JWT | 手动新建下载任务（磁力链接 / Torrent URL） |
| `POST` | `/api/downloads/pause-all` | JWT | 全部暂停 |
| `POST` | `/api/downloads/resume-all` | JWT | 全部继续 |
| `DELETE` | `/api/downloads/{hash}` | JWT | 删除任务（可选同时删除文件） |
| `POST` | `/api/downloads/{hash}/pause` | JWT | 暂停单个任务 |
| `POST` | `/api/downloads/{hash}/resume` | JWT | 继续单个任务 |
| `POST` | `/api/downloader/test` | JWT | 下载器连通性与时延测试 |

### 📖 设置、日志与自定义正则

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/settings` | JWT | 全部设置（敏感键返回空值） |
| `PUT` | `/api/settings` | JWT | 批量 Upsert 设置 |
| `GET` | `/api/settings/custom-regex` | JWT | 当前自定义正则规则 |
| `POST` | `/api/settings/custom-regex/reload` | JWT | 从数据库重载自定义正则 |
| `POST` | `/api/settings/custom-regex/test` | JWT | 正则匹配沙箱（实时回显解析结果） |
| `GET` | `/api/logs` | JWT | 系统日志（`?lines=`，默认 100，上限 500） |

### 备份与恢复

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/backup/list` | JWT | 备份文件列表 |
| `POST` | `/api/backup/create` | JWT | 创建数据库备份 |
| `POST` | `/api/backup/restore` | JWT | 从备份恢复（全字段覆盖写） |
| `DELETE` | `/api/backup/{name}` | JWT | 删除指定备份 |
| `GET` | `/api/backup/download/{name}` | JWT | 下载备份文件 |

### 通知投递监控

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `POST` | `/api/notify/test` | JWT | 指定渠道连通性测试 |
| `GET` | `/api/notifications/logs` | JWT | 投递履历（按渠道/状态/事件筛选 + 分页） |
| `DELETE` | `/api/notifications/logs` | JWT | 按保留天数清理履历 |
| `GET` | `/api/notifications/stats` | JWT | 投递总量、成功率、失败数与死信积压 |

### 解析与整理预检

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `POST` | `/api/rss/preview` | JWT | RSS 实时探针：抓取远程源并回显解析结果 |
| `POST` | `/api/organize/preview` | JWT | 整理 Dry-Run：预测目标路径、操作类型与冲突（零写入） |

### 📖 搜索与发现

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/search` | JWT | 聚合搜索番剧资源 |
| `POST` | `/api/search/smart` | JWT | 智能搜索（搜索词扩展 + 结果分页） |
| `GET` | `/api/mikan/groups` | JWT | 按 BangumiID 获取字幕组与 RSS 地址 |

### 时间表与 Bangumi 集成

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/schedule` | JWT | 季度新番时间表（`source=mikan` / `yuc`） |
| `GET` | `/api/schedule/bangumi` | JWT | Bangumi 每日放送日历 |
| `GET` | `/api/bangumi/subject/` | JWT | Bangumi 条目详情 |
| `GET` | `/api/bangumi/auth/link` | JWT | 生成 Bangumi OAuth 授权链接 |
| `GET` | `/api/bangumi/auth/callback` | 公开 | Bangumi OAuth 回调 |

### 📖 镜像测速与 AI

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `POST` | `/api/mikan/test-mirrors` | JWT | Mikan 镜像测速 |
| `POST` | `/api/bgm/test-mirrors` | JWT | Bangumi 镜像测速 |
| `POST` | `/api/mikan/select-mirror` | JWT | 选定 Mikan 镜像并持久化 |
| `POST` | `/api/bgm/select-mirror` | JWT | 选定 Bangumi 镜像并持久化 |
| `POST` | `/api/ai/models` | JWT | 拉取已配置 AI 端点的可用模型列表 |

### 📖 插件管理

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/plugins` | JWT | 插件总览（启用状态与关注事件） |
| `POST` | `/api/plugins/toggle` | JWT | 启用/停用插件 |
| `POST` | `/api/plugins/save` | JWT | 新增或更新自定义插件 |
| `DELETE` | `/api/plugins/{id}` | JWT | 删除自定义插件 |
| `POST` | `/api/plugins/reload` | JWT | 重载插件配置 |

### 📖 任务解析与数据迁移

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `POST` | `/api/parse` | JWT | 自然语言解析订阅任务（正则 + AI 回退） |
| `POST` | `/api/migrate` | JWT | 从 AutoBangumi / ani-rss 数据库导入数据 |

### 📖 图片代理与实时事件

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/api/proxy/image` | 公开 | 图片代理（域名白名单，绕过防盗链） |
| `GET` | `/api/events/stream` | JWT（可用 `?token=`） | SSE 实时事件流 |

### 📖 MCP 协议端点

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/mcp/sse` | MCP Token | MCP SSE 握手 |
| `POST` | `/mcp/messages` | MCP Token | MCP JSON-RPC 消息通道（`?session_id=`） |
| `POST` | `/api/mcp/token/generate` | JWT | 生成/重置 MCP Token |

---

## 快速开始

以下示例演示从登录到调用 API 的完整流程：

```bash
# 1. 登录获取 Token
TOKEN=$(curl -s -X POST http://localhost:20001/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"123456"}' | jq -r '.token')

# 2. 健康检查（无需认证）
curl http://localhost:20001/api/health

# 3. 获取订阅列表
curl -H "Authorization: Bearer $TOKEN" http://localhost:20001/api/subscriptions

# 4. 创建订阅
curl -X POST http://localhost:20001/api/subscriptions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"title_cn":"某科学的超电磁炮","bangumi_id":"12345"}'

# 5. 搜索番剧资源
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:20001/api/search?q=Railgun"

# 6. 获取新番时间表
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:20001/api/schedule?year=2026&season=2"

# 7. 查看系统日志
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:20001/api/logs?lines=20"
```

> **提示**: 将 `localhost:20001` 替换为实际部署地址。JWT 密钥持久化于数据库，重启后旧 Token 在 24 小时有效期内仍然可用；修改密码后旧 Token 立即失效。

---

## 认证机制

### 中间件链

```
请求 → ProxyHeadersMiddleware → CORSMiddleware → AuthMiddleware → Handler
```

### 认证规则

| 路径 | 认证要求 |
|------|---------|
| `POST /api/login` | ❌ 无需认证 |
| `GET /api/health` | ❌ 无需认证 |
| `GET /api/proxy/image` | ❌ 无需认证 |
| `GET /api/user/avatar`、`HEAD /api/user/avatar` | ❌ 无需认证（仅读取头像） |
| `GET /api/bangumi/auth/callback` | ❌ 无需认证（Bangumi OAuth 回调） |
| `GET /mcp/sse`、`POST /mcp/messages` | 🔑 需 MCP Bearer Token（不走 JWT 中间件） |
| 其他 `/api/*` | ✅ 需要 JWT Token |

### Token 使用方式

```http
Authorization: Bearer <your-jwt-token>
```

### Token 获取

通过 `POST /api/login` 登录获取。Token 为 HS256 签名的 JWT，有效期 **24 小时**，载荷包含用户名与 `token_version`；密钥在首次启动时由 `crypto/rand` 生成 32 字节并持久化于 settings 表的 `jwt_secret` 键，因此重启不会使 Token 失效，而修改密码会递增 `token_version` 使旧 Token 立即失效。

---

## 1. 认证与鉴权

### POST /api/login

用户登录，获取 JWT Token。

**请求参数**: Body (JSON)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `username` | string | ✅ | 用户名 |
| `password` | string | ✅ | 密码 |

**请求示例**:
```json
{
  "username": "admin",
  "password": "123456"
}
```

**curl 示例**:
```bash
curl -X POST http://localhost:20001/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"123456"}'
```

**响应格式**:
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "username": "admin",
  "message": "登录成功"
}
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | 请求格式错误 / 用户名和密码不能为空 |
| 401 | 用户名或密码错误 |
| 500 | Token 生成失败 |

---

### POST /api/user/change-password

修改当前用户密码。修改后所有旧 Token 失效。

**认证**: ✅ 需要 JWT Token

**请求参数**: Body (JSON)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `old_password` | string | ✅ | 旧密码 |
| `new_password` | string | ✅ | 新密码（至少6位） |

**请求示例**:
```json
{
  "old_password": "123456",
  "new_password": "newpass123"
}
```

**curl 示例**:
```bash
curl -X POST http://localhost:20001/api/user/change-password \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"old_password":"123456","new_password":"newpass123"}'
```

**响应示例**:
```json
{
  "message": "密码修改成功"
}
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | 请求格式错误 / 密码不能为空 / 新密码不能少于6位 / 新旧密码相同 |
| 401 | 未登录 |
| 403 | 旧密码错误 |
| 404 | 用户不存在 |
| 500 | 密码加密失败 / 保存失败 |

---

## 2. 健康检查

### GET /api/health

服务健康检查，无需认证。

**curl 示例**:
```bash
curl http://localhost:20001/api/health
```

**响应示例**:
```json
{
  "status": "ok",
  "time": "2025-01-15T10:30:00+08:00"
}
```

---

## 3. 用户信息

### GET /api/me

获取当前登录用户信息。

**认证**: ✅ 需要 JWT Token

**curl 示例**:
```bash
curl -H "Authorization: Bearer *** http://localhost:20001/api/me
```

**响应示例**:
```json
{
  "username": "admin"
}
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 401 | Token 无效 |

---

## 4. 订阅管理

### GET /api/subscriptions

获取所有订阅列表。

**认证**: ✅ 需要 JWT Token

**请求参数**: Query

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `enabled` | boolean | ❌ | 按启用状态过滤 |
| `completed` | boolean | ❌ | 按完成状态过滤 |

**请求示例**:
```
GET /api/subscriptions?enabled=true&completed=false
```

**curl 示例**:
```bash
# 获取所有订阅
curl -H "Authorization: Bearer YOUR_TOKEN" http://localhost:20001/api/subscriptions

# 按状态过滤
curl -H "Authorization: Bearer YOUR_TOKEN" \
  "http://localhost:20001/api/subscriptions?enabled=true&completed=false"
```

**响应示例**:
```json
[
  {
    "id": 1,
    "title_cn": "某科学的超电磁炮",
    "title_en": "A Certain Scientific Railgun",
    "title_jp": "とある科学の超電磁砲",
    "year": 2025,
    "season": 1,
    "bangumi_id": "12345",
    "subgroup_name": "字幕组A",
    "metadata_id": "tmdb:12345",
    "metadata_provider": "tmdb",
    "cover_url": "https://example.com/cover.jpg",
    "description": "...",
    "anime_type": "TV",
    "total_episodes": 24,
    "current_episodes": 12,
    "enabled": true,
    "completed": false,
    "filter_json": "{}",
    "custom_path": "",
    "stalled_episodes": 0,
    "created_at": "2025-01-01T00:00:00Z",
    "updated_at": "2025-01-15T10:00:00Z"
  }
]
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 500 | 查询订阅失败 |

---

### POST /api/subscriptions

创建新订阅。

**认证**: ✅ 需要 JWT Token

**请求参数**: Body (JSON)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `title_cn` | string | ✅ | 番剧中文标题 |
| `bangumi_id` | string | ❌ | Bangumi 番剧 ID |
| `subgroup_name` | string | ❌ | 字幕组名称 |
| `rss_url` | string | ❌ | RSS 订阅地址 |
| `filter_json` | string | ❌ | 过滤规则 JSON |
| `custom_path` | string | ❌ | 自定义下载路径 |
| `cover_url` | string | ❌ | 封面图片 URL |

**请求示例**:
```json
{
  "title_cn": "某科学的超电磁炮",
  "bangumi_id": "12345",
  "subgroup_name": "字幕组A",
  "cover_url": "https://example.com/cover.jpg"
}
```

**curl 示例**:
```bash
curl -X POST http://localhost:20001/api/subscriptions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer *** \
  -d '{"title_cn":"某科学的超电磁炮","bangumi_id":"12345","subgroup_name":"字幕组A","cover_url":"https://example.com/cover.jpg"}'
```

**响应示例**:
```json
{
  "id": 1,
  "title_cn": "某科学的超电磁炮",
  "title_en": "",
  "title_jp": "",
  "year": 0,
  "season": 0,
  "bangumi_id": "12345",
  "subgroup_name": "字幕组A",
  "metadata_id": "",
  "metadata_provider": "",
  "cover_url": "https://example.com/cover.jpg",
  "description": "",
  "anime_type": "",
  "total_episodes": 0,
  "current_episodes": 0,
  "enabled": true,
  "completed": false,
  "filter_json": "",
  "custom_path": "",
  "stalled_episodes": 0,
  "created_at": "2025-01-15T10:00:00Z",
  "updated_at": "2025-01-15T10:00:00Z"
}
```

**行为说明**:
- 创建后自动触发后台补全扫描（如果配置了调度器）
- 如果提供了 `bangumi_id` 但未提供 `rss_url`，后台会自动解析 Mikan 字幕组 RSS

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | 请求格式错误 / 番剧标题不能为空 |
| 500 | 创建订阅失败 |

---

### POST /api/subscriptions/batch

批量创建订阅（最多 20 部）。

**认证**: ✅ 需要 JWT Token

**请求参数**: Body (JSON)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `items` | array | ✅ | 订阅项数组 |

每个 `item`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `title_cn` | string | ✅ | 番剧中文标题 |
| `bangumi_id` | string | ❌ | Bangumi ID |
| `cover_url` | string | ❌ | 封面 URL |
| `subgroups` | string[] | ❌ | 字幕组列表 |

**请求示例**:
```json
{
  "items": [
    {
      "title_cn": "某科学的超电磁炮",
      "bangumi_id": "12345",
      "subgroups": ["字幕组A", "字幕组B"]
    },
    {
      "title_cn": "进击的巨人",
      "bangumi_id": "67890"
    }
  ]
}
```

**curl 示例**:
```bash
curl -X POST http://localhost:20001/api/subscriptions/batch \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer *** \
  -d '{"items":[{"title_cn":"某科学的超电磁炮","bangumi_id":"12345","subgroups":["字幕组A","字幕组B"]},{"title_cn":"进击的巨人","bangumi_id":"67890"}]}'
```

**响应示例**:
```json
{
  "success": [
    { "title": "某科学的超电磁炮", "id": 1 }
  ],
  "failed": [
    { "title": "进击的巨人", "error": "已存在订阅" }
  ]
}
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | 请求格式错误 / 批量订阅最多20部 / 未提供任何订阅项 |

---

### GET /api/subgroups

获取系统中已有的字幕组列表。

**认证**: ✅ 需要 JWT Token

**curl 示例**:
```bash
curl -H "Authorization: Bearer *** http://localhost:20001/api/subgroups
```

**响应示例**:
```json
["字幕组A", "字幕组B", "字幕组C"]
```

---

### GET /api/subscriptions/{id}

获取单个订阅详情，包含剧集列表。

**认证**: ✅ 需要 JWT Token

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| `id` | uint | 订阅 ID |

**curl 示例**:
```bash
curl -H "Authorization: Bearer *** http://localhost:20001/api/subscriptions/1
```

**响应示例**:
```json
{
  "subscription": {
    "id": 1,
    "title_cn": "某科学的超电磁炮",
    "title_en": "A Certain Scientific Railgun",
    "title_jp": "とある科学の超電磁砲",
    "year": 2025,
    "season": 1,
    "bangumi_id": "12345",
    "subgroup_name": "字幕组A",
    "metadata_id": "",
    "metadata_provider": "",
    "cover_url": "https://example.com/cover.jpg",
    "description": "...",
    "anime_type": "TV",
    "total_episodes": 24,
    "current_episodes": 12,
    "enabled": true,
    "completed": false,
    "filter_json": "{}",
    "custom_path": "",
    "stalled_episodes": 2,
    "created_at": "2025-01-01T00:00:00Z",
    "updated_at": "2025-01-15T10:00:00Z"
  },
  "episodes": [
    {
      "id": 1,
      "subscription_id": 1,
      "season": 1,
      "number": 1,
      "title": "第1话",
      "status": "completed",
      "torrent_hash": "abc123",
      "torrent_url": "https://...",
      "original_name": "[SubGroup] Title - 01.mkv",
      "final_path": "/downloads/某科学的超电磁炮/S01E01.mkv",
      "file_size": 350000000,
      "is_stalled": false,
      "group_name": "字幕组A",
      "download_started_at": "2025-01-02T00:00:00Z",
      "created_at": "2025-01-01T00:00:00Z"
    }
  ]
}
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | 无效的订阅 ID |
| 404 | 订阅不存在 |

---

### PUT /api/subscriptions/{id}

更新订阅信息（部分更新，仅更新提供的字段）。

**认证**: ✅ 需要 JWT Token

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| `id` | uint | 订阅 ID |

**请求参数**: Body (JSON)，所有字段均可选

| 字段 | 类型 | 说明 |
|------|------|------|
| `title_cn` | string | 中文标题 |
| `title_en` | string | 英文标题 |
| `title_jp` | string | 日文标题 |
| `year` | int | 年份 |
| `season` | int | 季度 |
| `bangumi_id` | string | Bangumi ID |
| `subgroup_name` | string | 字幕组 |
| `metadata_id` | string | 元数据 ID |
| `metadata_provider` | string | 元数据提供者 |
| `cover_url` | string | 封面 URL |
| `description` | string | 描述 |
| `anime_type` | string | 动画类型 |
| `total_episodes` | int | 总集数 |
| `enabled` | boolean | 是否启用 |
| `completed` | boolean | 是否完成 |
| `filter_json` | string | 过滤规则 |
| `custom_path` | string | 自定义路径 |

**请求示例**:
```json
{
  "enabled": false,
  "total_episodes": 24
}
```

**curl 示例**:
```bash
curl -X PUT http://localhost:20001/api/subscriptions/1 \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer *** \
  -d '{"enabled":false,"total_episodes":24}'
```

**响应示例**: 返回更新后的完整订阅对象（同 `GET /api/subscriptions/{id}` 的 `subscription` 部分）

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | 无效的订阅 ID / 未提供任何更新字段 |
| 404 | 订阅不存在 |
| 500 | 更新订阅失败 |

---

### DELETE /api/subscriptions/{id}

删除订阅及其关联剧集。

**认证**: ✅ 需要 JWT Token

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| `id` | uint | 订阅 ID |

**请求参数**: Query

| 参数 | 类型 | 说明 |
|------|------|------|
| `delete_files` | boolean | 是否同时删除下载的文件和种子 |

**请求示例**:
```
DELETE /api/subscriptions/1?delete_files=true
```

**curl 示例**:
```bash
# 删除订阅（保留文件）
curl -X DELETE http://localhost:20001/api/subscriptions/1 \
  -H "Authorization: Bearer ***

# 删除订阅并删除文件和种子
curl -X DELETE "http://localhost:20001/api/subscriptions/1?delete_files=true" \
  -H "Authorization: Bearer ***

**响应示例**:
```json
{
  "message": "订阅已删除"
}
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | 无效的订阅 ID |
| 404 | 订阅不存在 |

---

### POST /api/subscriptions/batch-delete

批量删除订阅（最多 100 个，使用事务）。

**认证**: ✅ 需要 JWT Token

**请求参数**: Body (JSON)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `ids` | uint[] | ✅ | 订阅 ID 列表 |
| `delete_files` | boolean | ❌ | 是否删除文件和种子 |

**请求示例**:
```json
{
  "ids": [1, 2, 3],
  "delete_files": true
}
```

**curl 示例**:
```bash
curl -X POST http://localhost:20001/api/subscriptions/batch-delete \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer *** \
  -d '{"ids":[1,2,3],"delete_files":true}'
```

**响应示例**:
```json
{
  "deleted": 3
}
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | 无效的请求体 / ID列表不能为空 / 单次最多删除100个 |
| 500 | 删除失败 |

---

### POST /api/subscriptions/batch-restore

批量恢复已软删除的订阅（最多 100 个）。

**认证**: ✅ 需要 JWT Token

**请求参数**: Body (JSON)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `ids` | uint[] | ✅ | 订阅 ID 列表 |

**请求示例**:
```json
{
  "ids": [1, 2, 3]
}
```

**curl 示例**:
```bash
curl -X POST http://localhost:20001/api/subscriptions/batch-restore \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer *** \
  -d '{"ids":[1,2,3]}'
```

**响应示例**:
```json
{
  "restored": 3
}
```

---

### POST /api/subscriptions/{id}/trigger-supplement

手动触发单个订阅的补全扫描。

**认证**: ✅ 需要 JWT Token

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| `id` | uint | 订阅 ID |

**curl 示例**:
```bash
curl -X POST http://localhost:20001/api/subscriptions/1/trigger-supplement \
  -H "Authorization: Bearer *** {
  "message": "补全任务已触发，将在后台执行"
}
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | 无效的订阅 ID / 订阅未启用 |
| 404 | 订阅不存在 |
| 500 | 补全调度器未配置 |

---

## 5. 剧集管理

### PUT /api/episodes/{id}/status

手动更新剧集状态。

**认证**: ✅ 需要 JWT Token

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| `id` | uint | 剧集 ID |

**请求参数**: Body (JSON)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `status` | string | ✅ | 状态值：`pending` / `downloading` / `completed` / `failed` |

**请求示例**:
```json
{
  "status": "completed"
}
```

**curl 示例**:
```bash
curl -X PUT http://localhost:20001/api/episodes/1/status \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer *** \
  -d '{"status":"completed"}'
```

**响应示例**:
```json
{
  "status": "completed"
}
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | 无效的剧集 ID / 请求格式错误 / 无效的状态值 |
| 500 | 更新失败 |

---

## 6. 下载队列

### GET /api/downloads

获取当前下载队列。

**认证**: ✅ 需要 JWT Token

**curl 示例**:
```bash
curl -H "Authorization: Bearer *** http://localhost:20001/api/downloads
```

**响应示例**:
```json
[
  {
    "hash": "abc123def456",
    "name": "[SubGroup] Anime Title - 01.mkv",
    "save_path": "/downloads/anime",
    "status": "downloading",
    "progress": 0.75,
    "speed_down": 2048000,
    "size": 350000000,
    "done": 262500000
  }
]
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 503 | 下载器未配置 |
| 500 | 获取下载列表失败 |

---

## 7. 设置管理

### GET /api/settings

获取所有系统设置。敏感字段（含 `PASS`/`SECRET`/`KEY` 的键名）返回空值。

**认证**: ✅ 需要 JWT Token

**curl 示例**:
```bash
curl -H "Authorization: Bearer *** http://localhost:20001/api/settings
```

**响应示例**:
```json
{
  "download_path": "/downloads",
  "stall_timeout_hours": "48",
  "mikan_domain": "mikanime.tv",
  "qb_password": ""
}
```

---

### PUT /api/settings

批量更新设置（Upsert 模式）。

**认证**: ✅ 需要 JWT Token

**请求参数**: Body (JSON)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `settings` | object | ✅ | 键值对，key 为设置名，value 为设置值 |

**请求示例**:
```json
{
  "settings": {
    "download_path": "/new/downloads",
    "stall_timeout_hours": "72"
  }
}
```

**curl 示例**:
```bash
curl -X PUT http://localhost:20001/api/settings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer *** \
  -d '{"settings":{"download_path":"/new/downloads","stall_timeout_hours":"72"}}'
```

**响应示例**:
```json
{
  "message": "设置已更新"
}
```

---

### GET /api/settings/custom-regex

获取当前自定义正则规则。

**认证**: ✅ 需要 JWT Token

**curl 示例**:
```bash
curl -H "Authorization: Bearer *** http://localhost:20001/api/settings/custom-regex
```

**响应示例**:
```json
{
  "patterns": ["[\\[\\【].*[\\]\\】].*\\d+"],
  "compiled": ["[\\[\\【].*[\\]\\】].*\\d+"],
  "builtin_count": 8
}
```

---

### POST /api/settings/custom-regex/reload

从数据库重新加载自定义正则规则。

**认证**: ✅ 需要 JWT Token

**curl 示例**:
```bash
curl -X POST http://localhost:20001/api/settings/custom-regex/reload \
  -H "Authorization: Bearer *** {
  "message": "自定义正则已重新加载",
  "compiled": ["pattern1", "pattern2"]
}
```

---

### GET /api/logs

获取系统日志（逆向读取，类似 `tail -n`）。

**认证**: ✅ 需要 JWT Token

**请求参数**: Query

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `lines` | int | ❌ | 返回行数，默认 100，最大 500 |

**请求示例**:
```
GET /api/logs?lines=50
```

**curl 示例**:
```bash
curl -H "Authorization: Bearer ***   "http://localhost:20001/api/logs?lines=50"
```

**响应示例**:
```json
{
  "lines": [
    "2025/01/15 10:30:00 ✅ 用户 admin 登录成功",
    "2025/01/15 10:31:00 🔍 搜索完成 [某科学的超电磁炮]: 找到 15 个结果"
  ],
  "total": 1024
}
```

---

## 8. 搜索与发现

### GET /api/search

搜索番剧资源（聚合 Nyaa/ACG.RIP/AnimeTosho）。

**认证**: ✅ 需要 JWT Token

**请求参数**: Query

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `q` | string | ✅ | 搜索关键词 |

**请求示例**:
```
GET /api/search?q=某科学的超电磁炮
```

**curl 示例**:
```bash
curl -H "Authorization: Bearer ***   "http://localhost:20001/api/search?q=%E6%9F%90%E7%A7%91%E5%AD%A6%E7%9A%84%E8%B6%85%E7%94%B5%E7%A3%81%E7%82%AE"
```

**响应示例**:
```json
[
  {
    "title": "[SubGroup] To Aru Kagaku no Railgun - 01 [1080p].mkv",
    "url": "https://nyaa.si/view/12345",
    "magnet": "magnet:?xt=urn:btih:abc123...",
    "info_hash": "abc123def456",
    "size": 350000000,
    "pub_date": "2025-01-01T00:00:00Z",
    "source": "nyaa",
    "bangumi_id": "",
    "episode_url": "",
    "cover_url": "",
    "aired_time": "",
    "aired_date": "",
    "group_name": "SubGroup"
  }
]
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | 搜索关键词不能为空 |
| 503 | 搜索服务未配置 |
| 500 | 搜索失败 |

---

### GET /api/mikan/groups

根据 BangumiID 获取 Mikan 字幕组列表。

**认证**: ✅ 需要 JWT Token

**请求参数**: Query

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `bangumi_id` | string | ✅ | Bangumi 番剧 ID |

**请求示例**:
```
GET /api/mikan/groups?bangumi_id=12345
```

**curl 示例**:
```bash
curl -H "Authorization: Bearer ***   "http://localhost:20001/api/mikan/groups?bangumi_id=12345"
```

**响应示例**:
```json
[
  {
    "name": "字幕组A",
    "rss_url": "https://mikanime.tv/RSS/Bangumi?bangumiId=12345&subgroupid=678",
    "subgroup_id": "678"
  }
]
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | bangumi_id 不能为空 |
| 503 | Mikan 服务未初始化 |
| 500 | 获取字幕组失败 |

---

### GET /api/schedule

获取季度新番时间表（默认使用蜜柑计划 Mikan 数据源；开启 `yuc_schedule` 插件后支持通过 `source=yuc` 切换至 yuc.wiki 数据源）。

**认证**: ✅ 需要 JWT Token

**请求参数**: Query

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `year` | int | ❌ | 年份（默认当前年，如 `2024`） |
| `season` | int | ❌ | 季度 1-4（默认当前季度，1:冬, 2:春, 3:夏, 4:秋） |
| `source` | string | ❌ | 数据源：`mikan`（默认）或 `yuc`（需启用 `yuc_schedule` 插件） |

**请求示例**:
```
GET /api/schedule?year=2025&season=1&source=mikan
```

**curl 示例**:
```bash
curl -H "Authorization: Bearer ***" "http://localhost:20001/api/schedule?year=2026&season=2"
```

---

### GET /api/schedule/bangumi

获取 Bangumi 放送时间表。当前季度返回实时每日放送日历；指定历史/未来年份和季度时，自动同步对应季度的放送列表。

**认证**: ✅ 需要 JWT Token

**请求参数**: Query

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `year` | int | ❌ | 年份（如 `2024`，默认当前年） |
| `season` | int | ❌ | 季度 1-4（默认当前季度） |

**请求示例**:
```
GET /api/schedule/bangumi?year=2024&season=4
```

**curl 示例**:
```bash
curl -H "Authorization: Bearer ***" "http://localhost:20001/api/schedule/bangumi?year=2024&season=4"
```

**响应示例**:
```json
{
  "days": [
    {
      "day_of_week": 1,
      "label": "周一",
      "items": [
        {
          "title": "某科学的超电磁炮",
          "bangumi_id": "12345",
          "info_hash": "1",
          "cover_url": "https://...",
          "aired_time": "23:00",
          "aired_date": "2025-01-06",
          "group_name": ""
        }
      ]
    }
  ],
  "subscribed": {
    "12345": 1
  },
  "subscriptionCount": 5,
  "sub_stats": {
    "1": {
      "downloaded": 12,
      "total": 24
    }
  }
}
```

**行为说明**:
- `info_hash` 字段在时间表中被复用为已订阅番剧的 subscription_id
- 自动匹配已订阅番剧（支持 BangumiID、标题、标准化标题模糊匹配）

---

## 9. Mikan 镜像管理

### POST /api/mikan/test-mirrors

测试所有 Mikan 镜像延迟。

**认证**: ✅ 需要 JWT Token

**curl 示例**:
```bash
curl -X POST http://localhost:20001/api/mikan/test-mirrors \
  -H "Authorization: Bearer ***...\"
```

**响应示例**:
```json
[
  {
    "domain": "mikanime.tv",
    "latency": 120,
    "ok": true
  },
  {
    "domain": "mikanani.me",
    "latency": 350,
    "ok": true
  }
]
```

---

### POST /api/mikan/select-mirror

选择 Mikan 镜像域名（保存到数据库）。

**认证**: ✅ 需要 JWT Token

**请求参数**: Body (JSON)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `domain` | string | ✅ | 镜像域名 |

**请求示例**:
```json
{
  "domain": "mikanime.tv"
}
```

**curl 示例**:
```bash
curl -X POST http://localhost:20001/api/mikan/select-mirror \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer *** \
  -d '{"domain":"mikanime.tv"}'
```

**响应示例**:
```json
{
  "domain": "mikanime.tv"
}
```

---

## 10. 插件管理

### GET /api/plugins

获取已加载的插件列表。

**认证**: ✅ 需要 JWT Token

**curl 示例**:
```bash
curl -H "Authorization: Bearer *** http://localhost:20001/api/plugins
```

**响应示例**:
```json
[
  {
    "name": "webhook-notify",
    "type": "webhook",
    "enabled": true
  }
]
```

---

### POST /api/plugins/reload

重新加载插件配置。

**认证**: ✅ 需要 JWT Token

**curl 示例**:
```bash
curl -X POST http://localhost:20001/api/plugins/reload \
  -H "Authorization: Bearer ***...\"
```

**响应示例**:
```json
{
  "message": "插件已重新加载",
  "count": 3
}
```

---

## 11. 任务解析

### POST /api/parse

自然语言解析订阅任务（正则 + AI 回退）。

**认证**: ✅ 需要 JWT Token

**请求参数**: Body (JSON)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `input` | string | ✅ | 自然语言指令 |

**请求示例**:
```json
{
  "input": "追番 某科学的超电磁炮 第一季 1080p"
}
```

**curl 示例**:
```bash
curl -X POST http://localhost:20001/api/parse \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer *** \
  -d '{"input":"追番 某科学的超电磁炮 第一季 1080p"}'
```

**响应示例**:
```json
{
  "action": "subscribe",
  "title": "某科学的超电磁炮",
  "season": 1,
  "resolution": "1080p",
  "subgroups": [],
  "confidence": 0.95
}
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | 请求格式错误 / 请输入指令 |
| 503 | 任务解析器未初始化 |
| 500 | 解析失败 |

---

## 12. 数据迁移

### POST /api/migrate

从 AutoBangumi / ani-rss SQLite 数据库迁移数据。

**认证**: ✅ 需要 JWT Token

**请求参数**: Body (JSON)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `source_path` | string | ✅ | 源数据库文件路径 |

**请求示例**:
```json
{
  "source_path": "data/anirss.db"
}
```

**curl 示例**:
```bash
curl -X POST http://localhost:20001/api/migrate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer *** \
  -d '{"source_path":"data/anirss.db"}'
```

**响应示例**:
```json
{
  "message": "迁移成功",
  "stats": {
    "subscriptions": 15,
    "episodes": 240
  }
}
```

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | 请求格式错误 / 请提供 source_path |
| 500 | 迁移失败 |

---

## 13. 版本信息

### GET /api/version

获取应用版本信息和更新日志。

**认证**: ✅ 需要 JWT Token

**curl 示例**:
```bash
curl -H "Authorization: Bearer *** http://localhost:20001/api/version
```

**响应示例**:
```json
{
  "version": "v0.6.0",
  "changelog": [
    "🏛️ 微内核与插件解耦架构：主干专注于五步极速流水线，周边生态全插拔，停用时 0 协程、0 网络、0 错误日志",
    "🧠 智能消歧与 Token 熔断保护：Bangumi 放送时间对齐（0 Token 化解 95% 歧义）、终身唯一 SQLite 解析缓存与每日限额熔断",
    "🤖 原生外部 AI 控制 (MCP Server)：标准 SSE + Bearer 强鉴权，暴露 6 项业务运维工具与 4 项系统治理工具",
    "🛡️ 做种生命周期与磁盘防爆急停：完成做种仅删任务记录永不删物理硬链接，剩余空间 < 5GB 自动熔断阻断新增下载",
    "👁️ 媒体整理 Dry-Run 预检：提供路径预览与冲突检测 API，无写入安全预测整理结果",
    "⚡ 下载器高可用与深度加固：Aria2 / Transmission URL 自动清洗、qB 超时防护、下载器连通性一键测试"
  ]
}
```

> 说明：`version` 取自编译期注入的 `main.version`（当前 `v0.6.0`）；`changelog` 为该版本的内置变更摘要，与 `CHANGELOG.md` 详细版本履历互补。

---

## 14. 图片代理

### GET /api/proxy/image

代理图片请求（绕过 CDN 热链保护），无需认证。

**请求参数**: Query

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `url` | string | ✅ | 图片原始 URL |

**允许的域名白名单**（与 `internal/api/handlers.go` 中的 `allowedDomains` 一致）:
- `i0.hdslb.com`、`bilibili.com`（Bilibili）
- `lain.bgm.tv`、`bgm.tv`（Bangumi）
- `img.mikanani.me`、`mikanani.me`、`mikanani.kas.pub`、`mikanime.tv`（Mikan 及镜像）
- `image.tmdb.org`（TMDB）
- `yuc.wiki`、`yucc.wiki`、`yucwiki.net`（長門番堂）

**请求示例**:
```
GET /api/proxy/image?url=https://i0.hdslb.com/bfs/archive/abc.jpg
```

**curl 示例**:
```bash
curl -o image.jpg \
  "http://localhost:20001/api/proxy/image?url=https://i0.hdslb.com/bfs/archive/abc.jpg"
```

**响应**: 直接返回图片二进制流，`Content-Type` 透传，缓存 7 天。

**错误码**:
| 状态码 | 说明 |
|--------|------|
| 400 | missing url / invalid url |
| 403 | domain not allowed |
| 502 | fetch failed |

---

## 15. MCP 协议端点

Ani-Go 内置原生 Model Context Protocol (MCP) Server，用于 Claude Desktop、Cursor、Dify 等外部 AI 智能体直接调用。

### 15.1 MCP SSE 建立连接

- **路径**: `GET /mcp/sse`（挂载于根路径，**不存在** `/api/mcp/sse`）
- **认证**: `Authorization: Bearer <MCP_TOKEN>`（或通过查询参数 `?token=<TOKEN>`）
- **内容类型**: `text/event-stream`
- **响应事件**:
  - `endpoint`: 返回客户端发送 JSON-RPC 消息的目标 POST URL（形如 `/mcp/messages?session_id=xxx`）。

### 15.2 MCP 消息通信

- **路径**: `POST /mcp/messages?session_id=<SESSION_ID>`
- **认证**: `Authorization: Bearer <MCP_TOKEN>`
- **请求体**: 标准 JSON-RPC 2.0 格式（`initialize`, `tools/list`, `tools/call`, `resources/list` 等）。
- **可用工具集 (Tools，固定 10 项)**:
  - `search_anime`: 搜索番剧资源（`keyword` 必填，`source` 可选）
  - `list_subscriptions`: 查询所有订阅与进度（`enabled_only` 可选）
  - `add_subscription`: 添加新的番剧订阅（`title` 必填，`season` / `rss_url` / `subgroup` 可选）
  - `remove_subscription`: 取消番剧订阅（`subscription_id` 必填，不删除已硬链接的媒体文件）
  - `get_downloads`: 获取下载队列与进度（`status` 可选：all / downloading / completed）
  - `trigger_refresh`: 触发 RSS 检查与全量扫描
  - `list_plugins`: 查询所有插件状态与关注事件
  - `toggle_plugin`: 启用或停用插件（`plugin_id`、`enabled`）
  - `get_system_status`: 查询内存、Goroutine、下载器连通性、订阅数与版本
  - `update_system_config`: 写入系统设置项（`key`、`value`）

> 当前版本**不支持插件动态挂载私有 MCP 工具**，工具集恒为上述 10 项，详见 `docs/02_PLUGIN_SPEC.md` 第 6 节。
>
> MCP Token 的生成/重置端点为 `POST /api/mcp/token/generate`（JWT 鉴权）。

---

## 16. 前端 API 调用参考

前端使用 `axios` 封装，基础路径 `/api`，自动附加 JWT Token。

### 认证相关

```typescript
// 登录
request.post('/login', { username, password })

// 获取当前用户
request.get('/me')

// 修改密码
request.post('/user/change-password', { old_password, new_password })
```

### 订阅管理

```typescript
// 获取订阅列表
request.get('/subscriptions')

// 创建订阅
request.post('/subscriptions', { title_cn, bangumi_id, subgroup_name, cover_url })

// 批量创建
request.post('/subscriptions/batch', { items: [...] }, { timeout: 30000 })

// 获取订阅详情
request.get(`/subscriptions/${id}`)

// 更新订阅
request.put(`/subscriptions/${id}`, { enabled: !sub.enabled })

// 删除订阅
request.post('/subscriptions/batch-delete', { ids: [...], delete_files: true })

// 触发补全
request.post(`/subscriptions/${sub.id}/trigger-supplement`)
```

### 剧集管理

```typescript
// 更新剧集状态
request.put(`/episodes/${ep.id}/status`, { status: nextStatus })
```

### 搜索

```typescript
// 搜索番剧
request.get('/search', { params: { q: keyword } })

// 获取字幕组
request.get('/mikan/groups', { params: { bangumi_id } })
```

### 时间表

```typescript
// 获取时间表
request.get('/schedule', { params: { year, season } })
```

### 设置

```typescript
// 获取设置
request.get('/settings')

// 更新设置
request.put('/settings', { settings: changed })

// 获取日志
request.get('/logs', { params: { lines: 100 } })
```

### Mikan 镜像

```typescript
// 测试镜像
request.post('/mikan/test-mirrors', {}, { timeout: 15000 })

// 选择镜像
request.post('/mikan/select-mirror', { domain })
```

---

## 17. 错误码说明

### 通用错误响应格式

```json
{
  "error": "错误描述信息"
}
```

### HTTP 状态码

| 状态码 | 含义 | 常见场景 |
|--------|------|---------|
| 200 | OK | 成功 |
| 201 | Created | 创建成功（订阅） |
| 202 | Accepted | 异步任务已接受（补全扫描） |
| 400 | Bad Request | 参数缺失、格式错误、业务校验失败 |
| 401 | Unauthorized | Token 缺失、无效、过期、版本不匹配 |
| 403 | Forbidden | 旧密码错误、图片域名不在白名单 |
| 404 | Not Found | 订阅不存在、用户不存在 |
| 405 | Method Not Allowed | HTTP 方法不支持 |
| 500 | Internal Server Error | 服务端异常 |
| 502 | Bad Gateway | 图片代理获取失败 |
| 503 | Service Unavailable | 下载器/搜索服务/插件管理器未配置 |

---

## 附录：数据库模型

> 模型定义见 `internal/database/models.go`，共 7 个实体；所有实体均内嵌 `gorm.Model`（含 `id` / `created_at` / `updated_at` / `deleted_at` 软删除字段）。

### Subscription（订阅）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| title_cn | string | 中文标题 |
| title_en | string | 英文标题 |
| title_jp | string | 日文标题 |
| year | int | 年份 |
| season | int | 季度（默认 1） |
| source_name | string | 来源站点名 |
| bangumi_id | string | Bangumi ID |
| rss_url | string | RSS 地址 |
| subgroup_name | string | 字幕组 |
| subgroup_id | string | Mikan 字幕组 ID |
| excluded_keywords | text | 排除关键词 |
| allowed_subgroups | text | 允许的字幕组白名单（JSON 数组） |
| skip_bulk_update | bool | 批量补全时跳过该订阅 |
| metadata_id | string | 元数据 ID |
| metadata_provider | string | 元数据提供者 |
| cover_url | string | 封面 URL |
| description | text | 描述 |
| anime_type | string | 动画类型 |
| total_episodes | int | 总集数 |
| current_episodes | int | 当前集数 |
| enabled | *bool | 是否启用（指针类型，默认 true） |
| completed | bool | 是否完成 |
| filter_json | text | 过滤规则 |
| custom_path | string | 自定义路径 |
| series_id | string | 系列归并 ID |
| tmdb_id | string | TMDB ID（索引） |
| imdb_id | string | IMDB ID（索引） |
| stall_timeout_hours | int | 死种超时阈值（小时，默认 24） |
| created_at / updated_at / deleted_at | time | 时间戳与软删除 |

### Episode（剧集）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| subscription_id | uint | 订阅 ID（索引） |
| season | int | 季 |
| number | float32 | 集号（支持 .5 半集） |
| title | string | 标题 |
| status | string | 状态：pending / downloading / downloaded / completed / failed |
| torrent_hash | string | 种子哈希（唯一索引） |
| torrent_url | string | 种子 URL |
| original_name | string | 原始文件名 |
| final_path | string | 最终路径 |
| file_size | int64 | 文件大小 |
| group_name | string | 字幕组 |
| resolution | string | 分辨率 |
| download_started_at / download_finished_at / organized_at | *time.Time | 下载与整理时间戳 |

### DownloadRecord（下载去重）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| torrent_hash | string | 种子哈希（唯一索引，用于避免重复下发） |
| torrent_url | string | 种子 URL |
| source_name | string | 来源站点 |
| added_at | time | 记录时间 |

### Setting（设置）

| 字段 | 类型 | 说明 |
|------|------|------|
| key | string | 设置键名（主键） |
| value | text | 设置值 |

### User（用户）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| username | string | 用户名（唯一索引） |
| password_hash | string | Bcrypt 密码哈希 |
| token_version | int | Token 版本号（修改密码后递增） |
| avatar_url | string | 头像地址 |

### NotificationLog（通知投递履历）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| event_type | string | 事件类型（索引） |
| channel | string | 推送渠道（索引） |
| title | string | 通知标题 |
| content | text | 通知正文 |
| status | string | 投递状态：success / failed / dlq（索引） |
| error_msg | text | 错误信息 |
| retry_count | int | 重试次数 |

### ParserCache（标题解析缓存）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| raw_title | text | 原始种子标题（唯一索引，缓存主键） |
| clean_title | text | 清洗后标题 |
| season | int | 解析出的季号 |
| episode | float32 | 解析出的集号 |
| subgroup | string | 字幕组 |
| resolution | string | 分辨率 |
| confidence | float32 | 置信度 |
| resolved_by | string | 解析来源：regex / airdate / ai |
