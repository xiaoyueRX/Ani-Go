# Ani-Go API 文档

> **版本**: v0.5.1  
> **基础路径**: `/api`  
> **认证方式**: JWT Bearer Token  
> **内容类型**: `application/json; charset=utf-8`

---

## 目录

1. [快速开始](#快速开始)
2. [认证与鉴权](#1-认证与鉴权)
2. [健康检查](#2-健康检查)
3. [用户信息](#3-用户信息)
4. [订阅管理](#4-订阅管理)
5. [剧集管理](#5-剧集管理)
6. [下载队列](#6-下载队列)
7. [设置管理](#7-设置管理)
8. [搜索与发现](#8-搜索与发现)
9. [Mikan 镜像管理](#9-mikan-镜像管理)
10. [插件管理](#10-插件管理)
11. [任务解析](#11-任务解析)
12. [数据迁移](#12-数据迁移)
13. [版本信息](#13-版本信息)
14. [图片代理](#14-图片代理)
15. [前端 API 调用参考](#15-前端-api-调用参考)
16. [错误码说明](#16-错误码说明)

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

> **提示**: 将 `localhost:20001` 替换为实际部署地址。Token 重启后失效（动态生成密钥）。

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
| 其他 `/api/*` | ✅ 需要 JWT Token |

### Token 使用方式

```http
Authorization: Bearer <your-jwt-token>
```

### Token 获取

通过 `POST /api/login` 登录获取，Token 包含用户名和版本号，密码修改后旧 Token 自动失效。

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
  "version": "v1.3.0",
  "changelog": [
    "新增引导弹窗单次会话逻辑，不再频繁打扰",
    "新增版本更新日志提示，及时了解新功能",
    "新增自动检查更新功能，支持检测 GitHub 最新版本",
    "优化设置页布局，增加自动更新开关",
    "修复部分 UI 显示问题"
  ]
}
```

---

## 14. 图片代理

### GET /api/proxy/image

代理图片请求（绕过 CDN 热链保护），无需认证。

**请求参数**: Query

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `url` | string | ✅ | 图片原始 URL |

**允许的域名白名单**:
- `i0.hdslb.com` (Bilibili)
- `lain.bgm.tv` (Bangumi)
- `img.mikanani.me` (Mikan)
- `image.tmdb.org` (TMDB)
- `bilibili.com`
- `bgm.tv`
- `mikanime.tv`

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

## 15. 前端 API 调用参考

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

## 16. 错误码说明

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

### Subscription（订阅）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| title_cn | string | 中文标题 |
| title_en | string | 英文标题 |
| title_jp | string | 日文标题 |
| year | int | 年份 |
| season | int | 季度 |
| bangumi_id | string | Bangumi ID |
| subgroup_name | string | 字幕组 |
| rss_url | string | RSS 地址 |
| metadata_id | string | 元数据 ID |
| metadata_provider | string | 元数据提供者 |
| cover_url | string | 封面 URL |
| description | string | 描述 |
| anime_type | string | 动画类型 |
| total_episodes | int | 总集数 |
| current_episodes | int | 当前集数 |
| enabled | bool | 是否启用 |
| completed | bool | 是否完成 |
| filter_json | string | 过滤规则 |
| custom_path | string | 自定义路径 |
| created_at | time | 创建时间 |
| updated_at | time | 更新时间 |
| deleted_at | time | 软删除时间 |

### Episode（剧集）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| subscription_id | uint | 订阅 ID |
| season | int | 季 |
| number | float32 | 集号 |
| title | string | 标题 |
| status | string | 状态：pending/downloading/downloaded/completed/failed |
| torrent_hash | string | 种子哈希 |
| torrent_url | string | 种子 URL |
| original_name | string | 原始文件名 |
| final_path | string | 最终路径 |
| file_size | int64 | 文件大小 |
| group_name | string | 字幕组 |
| download_started_at | time | 下载开始时间 |
| created_at | time | 创建时间 |
| deleted_at | time | 软删除时间 |

### Setting（设置）

| 字段 | 类型 | 说明 |
|------|------|------|
| key | string | 设置键名 |
| value | string | 设置值 |

### User（用户）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| username | string | 用户名 |
| password_hash | string | Bcrypt 密码哈希 |
| token_version | int | Token 版本号（修改密码后递增） |
