# GoChat 微服务化与前端组件化设计

## 概述

将 GoChat 改造为可被外部系统集成的微服务组件。后端以 Docker 镜像方式提供聊天能力，前端以 Vue 组件库方式嵌入外部系统。

## 设计目标

1. **用户服务外置** — GoChat 不实现用户认证，通过调用外部 API 获取用户信息
2. **前端组件化** — 聊天组件可被任何 Vue 项目嵌入使用
3. **配置 12-factor** — 环境变量覆盖全部配置，适合容器化部署
4. **API 版本化** — 稳定可演化的 API 契约

---

## 架构总览

```
外部系统                          GoChat 微服务
┌─────────────────────┐          ┌──────────────────────┐
│ 外部前端             │          │ Gin API              │
│ (内嵌 ChatWidget)    │──REST/WS─►│ ├─ /api/v1/auth/     │
│                      │          │ ├─ /api/v1/message/  │
│ 外部用户服务          │          │ ├─ /api/v1/file/     │
│ GET /user/info       │◄──HTTP──│ └─ /api/v1/group/    │
│ 返回 {id,name}      │  调用    │                      │
└─────────────────────┘          │ WebSocket /ws         │
                                 │                      │
                                 │ MySQL / Redis / MinIO │
                                 └──────────────────────┘
```

---

## 第一章：认证打通

### 核心思路

GoChat 不实现用户认证。外部系统需要提供一个 **获取用户信息的 API**，GoChat 在认证时调用此 API。

### 外部系统需实现的 API

```http
GET /api/user/info
Authorization: Bearer <用户token>
```

> **响应格式**
```json
{
  "id": "42",
  "name": "张三",
  "avatar": "https://..."
}
```

GoChat 只关心三个字段：
- `id` — 用户唯一标识（字符串）
- `name` — 显示名称
- `avatar` — 头像 URL（可选）

### 认证流程

```
前端(已有外部JWT)          GoChat                      外部系统
  │                         │                            │
  │── POST /api/v1/auth ───►│                            │
  │   Authorization:        │                            │
  │   Bearer <外部JWT>      │                            │
  │                         │                            │
  │                         ├── GET /user/info ────────► │
  │                         │   Authorization: Bearer xx │
  │                         │◄── { id:"42",name:"张三"} ─┤
  │                         │                            │
  │                         ├─ FindOrCreate user 记录     │
  │                         │   (id=42, name=张三)        │
  │                         ├─ 生成 gochat_token          │
  │◄── { token, user } ─────┤                            │
```

### 配置项

```yaml
auth:
  mode: external                 # builtin | external
  external_user_api: "http://external-system/api/user/info"
```

切换 `mode: builtin` 时使用 GoChat 自带的注册/登录功能（当前实现），`mode: external` 时使用上述流程。

### GoChat user 表

**不改任何表结构**。使用现有 `user` 表的字段：

| 字段 | 用途 |
|------|------|
| `username` | `ext_{外部ID}` — 外部用户标识，避免与注册用户冲突 |
| `nickname` | 外部返回的显示名称 |
| `avatar` | 头像 URL |

认证时按 `username = ext_{id}` 查找，不存在则创建。

---

## 第二章：API 版本化

当前路由注册 `/api/v1/` 前缀，旧路由通过重定向兼容：

```go
v1 := engine.Group("/api/v1")
{
    auth := v1.Group("/auth")
    {
        auth.POST("/auth", authHandler.ExternalAuth)  // 外部认证
    }
    // ... 其他路由不变
}

// 兼容旧路由
api := engine.Group("/api")
api.Any("/*path", redirectToV1)
```

### 端点清单

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| POST | `/api/v1/auth` | 外部认证（调外部 API 获取用户） | 无 |
| POST | `/api/v1/auth/login` | 内置登录（mode=builtin） | 无 |
| POST | `/api/v1/auth/register` | 内置注册（mode=builtin） | 无 |
| REST | 消息/好友/群组/上传 | 不变 | JWT |
| GET | `/healthz` | 健康检查 | 无 |

---

## 第三章：前端组件化

### 包结构

```
web/src/
├── components/
│   ├── ChatWidget.vue       ← 可嵌入的核心聊天组件
│   └── ContactList.vue      ← 联系人列表
├── composables/
│   └── useChat.js           ← 封装的聊天逻辑
├── api/ws.js                ← WebSocket 客户端
├── api/http.js              ← HTTP 客户端
├── App.vue                  ← 独立应用入口（保留）
└── main.js                  ← 独立应用入口（保留）
```

### ChatWidget 组件

```vue
<template>
  <div class="gochat-widget">
    <contact-list :items="contacts" @select="openChat" />
    <chat-panel :messages="messages" @send="handleSend" />
  </div>
</template>

<script setup>
const props = defineProps({
  apiBaseUrl: { type: String, default: '/api/v1' },
  token: { type: String, required: true },       // gochat_token
  userId: { type: String, required: true },       // 用户ID
  contacts: { type: Array, default: () => [] }    // [{id, name, avatar}]
})

const emit = defineEmits(['message-sent', 'message-received'])
</script>

<style scoped>
/* 样式隔离，不影响外部系统 */
</style>
```

### 嵌入方式

```javascript
import ChatWidget from '@gochat/vue-chat'

// 外部系统获取 gochat_token 后
;<chat-widget
  :token="gochatToken"
  :userId="currentUser.id"
  :contacts="friendList"
/>
```

---

## 第四章：配置 12-factor

所有配置通过环境变量覆盖：

```yaml
server:
  api_addr: "${API_ADDR::8080}"
  connect_addr: "${CONNECT_ADDR::7000}"

auth:
  mode: "${AUTH_MODE:builtin}"
  external_user_api: "${AUTH_EXTERNAL_API}"

mysql:
  host: "${MYSQL_HOST:localhost}"
  port: ${MYSQL_PORT:3306}
  database: "${MYSQL_DB:gochat}"

minio:
  endpoint: "${MINIO_ENDPOINT:localhost:9000}"
  access_key_id: "${MINIO_ACCESS_KEY}"
  bucket: "${MINIO_BUCKET:gochat}"
```

---

## 第五章：实施计划

### Phase 1: 外部认证

- `user` 表加 `external_id` 字段
- 实现 `POST /api/v1/auth`（调用外部 API 获取用户）
- `auth.mode` 配置切换（builtin / external）
- 前端登录页适配两种模式

### Phase 2: API 版本化

- 注册 `/api/v1/` 路由组
- 添加 `/healthz` 端点
- 兼容旧路由

### Phase 3: 前端组件化

- 提取 `ChatWidget.vue` 组件
- 实现 props/events API
- `useChat` composable 封装 WS 连接和消息状态

### Phase 4: 文档与示例

- 集成文档
- 外部系统对接示例（含外部 API 实现要求）
