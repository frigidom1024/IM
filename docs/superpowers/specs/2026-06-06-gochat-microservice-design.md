# GoChat 微服务化与前端组件化设计

## 概述

将 GoChat 改造为可被外部系统集成的微服务组件。后端以 Docker 镜像方式提供聊天能力，前端以 Vue 组件库方式嵌入外部系统。

## 设计目标

1. **零用户表依赖** — 外部系统通过 Token 交换使用 GoChat，不共享用户表
2. **前端组件化** — 聊天组件可被任何 Vue 项目 `npm install` 后使用
3. **配置 12-factor** — 环境变量覆盖全部配置，适合容器化部署
4. **API 版本化** — 稳定可演化的 API 契约

---

## 架构总览

```
外部系统                          GoChat 微服务
┌─────────────┐                  ┌──────────────────────┐
│ 外部前端     │                  │ Gin API              │
│ (Vue/React) │── REST / WS ──►  │ ├─ /api/v1/auth/     │
│             │                  │ ├─ /api/v1/message/  │
│ 嵌入         │                  │ ├─ /api/v1/file/     │
│ ChatWidget  │                  │ └─ /api/v1/group/    │
│             │                  │                      │
│ @gochat/    │                  │ WebSocket /ws         │
│ vue-chat    │                  │                      │
└─────────────┘                  │ MinIO / MySQL / Redis │
                                 └──────────────────────┘
```

---

## 第一章：认证打通

### 核心思路

GoChat 不维护 `user` 表。外部系统通过 Token 交换接口传入用户身份，GoChat 创建轻量映射。

### 认证流程

```
外部前端                                GoChat
  │                                      │
  │ 1. 在外部系统登录 (已有外部JWT)       │
  │                                      │
  │ 2. POST /api/v1/auth/exchange ──────►│
  │    { token: "外部JWT" }              │
  │                                      ├─ 验证外部 JWT
  │                                      │   (公钥/密钥由配置提供)
  │                                      ├─ 解析 sub/external_id
  │                                      ├─ UPSERT external_user 映射
  │                                      ├─ 生成 gochat_token
  │◄── { gochat_token, user_info } ─────┤
  │                                      │
  │ 3. 用 gochat_token 初始化 ChatWidget │
  │    传给组件或存储到 localStorage     │
```

### 数据库：external_user 表

```sql
CREATE TABLE external_user (
    id           BIGINT PRIMARY KEY AUTO_INCREMENT,
    external_id  VARCHAR(64) NOT NULL,
    system       VARCHAR(32) NOT NULL,        -- 来源系统标识
    display_name VARCHAR(64) DEFAULT '',
    avatar_url   VARCHAR(255) DEFAULT '',
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_system_user (system, external_id)
);
```

- `external_id` — 外部系统中的用户 ID（字符串，避免类型冲突）
- `system` — 来源系统标识（如 `"scraper"`, `"erp"`），不同系统的 user_id 可以重复

### GoChat JWT 结构

```json
{
  "sub": "42",
  "ext_id": "42",
  "system": "scraper",
  "name": "张三",
  "exp": 1780917426
}
```

- `sub` — 映射后的 `external_user.id` (用于内部关联)
- `ext_id` — 外部用户 ID，消息的 `from_id` 字段存储此值
- `system` — 来源系统，用于多系统隔离

### 配置项

```yaml
auth:
  mode: external              # builtin | external
  external_jwks_url: ""       # 外部 JWT 公钥端点
  external_jwt_secret: ""     # 外部 JWT 密钥（对称加密）
  external_jwt_algorithm: RS256
```

---

## 第二章：消息模型改造

### from_id 改为字符串

当前 `model.Message` 用 `from_id BIGINT` 指向 `user.id`。微服务化后改为可变长字符串：

```go
type Message struct {
    ID      int64  `gorm:"column:id;primaryKey" json:"id,string"`
    FromID  string `gorm:"column:from_id;type:varchar(64);not null;index:idx_from" json:"from_id"`
    ToID    string `gorm:"column:to_id;type:varchar(64);default:'';index:idx_to" json:"to_id"`
    GroupID int64  `gorm:"column:group_id;default:0;index:idx_group" json:"group_id"`
    // ...
}
```

- 私聊时 `from_id` 和 `to_id` 存外部用户 ID（如 `"42"`）
- 群聊时 `from_id` 存发送者的外部用户 ID
- 前端的 `sender_name` 字段仍然通过 API 查询后填充

### API 响应扩展

```json
{
  "id": "321622086873387008",
  "from_id": "42",
  "msg_type": "text",
  "content": "你好",
  "sender_name": "张三",
  "sender_avatar": "",
  "system": "scraper"
}
```

---

## 第三章：API 版本化

当前路由 `POST /api/auth/login` 改为 `POST /api/v1/auth/exchange`。

兼容旧路径（通过路由重定向或同时注册两条路由）：

```go
v1 := engine.Group("/api/v1")
{
    auth := v1.Group("/auth")
    {
        auth.POST("/exchange", authHandler.Exchange)  // Token交换
    }
    message := v1.Group("/message")
    {
        message.GET("/private/:userId", messageHandler.GetPrivateMessages)
        message.GET("/group/:groupId", messageHandler.GetGroupMessages)
        message.POST("/read", messageHandler.MarkRead)
    }
    file := v1.Group("/file")
    {
        file.POST("/upload", uploadHandler.Upload)
    }
}

// 兼容旧路径
api := engine.Group("/api")
api.Any("/*path", func(c *gin.Context) {
    c.Redirect(http.StatusMovedPermanently, "/api/v1/"+c.Param("path"))
})
```

### 端点清单

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| POST | `/api/v1/auth/exchange` | Token 交换 | 无 |
| GET | `/api/v1/user/info` | 获取当前用户信息 | JWT |
| GET | `/api/v1/message/private/:userId` | 私聊消息 | JWT |
| GET | `/api/v1/message/group/:groupId` | 群聊消息 | JWT |
| POST | `/api/v1/message/read` | 标记已读 | JWT |
| POST | `/api/v1/file/upload` | 文件上传 | JWT |
| GET | `/api/v1/group/list` | 群组列表 | JWT |
| POST | `/api/v1/group/create` | 创建群组 | JWT |
| GET | `/healthz` | 健康检查 | 无 |
| GET | `/api/v1/version` | 版本信息 | 无 |

---

## 第四章：前端组件化

### 包结构

```
web/
├── src/
│   ├── components/
│   │   └── ChatWidget.vue    ← 核心可嵌入组件
│   ├── composables/
│   │   └── useChat.js        ← 可复用的聊天逻辑
│   ├── api/
│   │   └── ws.js             ← WebSocket 客户端（独立可复用）
│   ├── stores/
│   │   └── chat.js           ← Pinia store（可选）
│   ├── App.vue               ← 独立应用入口（保留）
│   └── main.js               ← 独立应用入口（保留）
├── package.json
└── vite.config.js
```

### ChatWidget 组件

```vue
<template>
  <div class="gochat-widget">
    <contact-list :friends="friends" @select="openChat" />
    <chat-panel :messages="messages" @send="sendMessage" />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { wsClient } from '../api/ws'

const props = defineProps({
  apiBaseUrl: { type: String, default: '/api/v1' },
  token: { type: String, required: true },
  userId: { type: String, required: true },
  friends: { type: Array, default: () => [] }
})

const emit = defineEmits(['message-sent', 'message-received'])

onMounted(() => {
  wsClient.connect(props.token)
})
</script>

<style scoped>
/* scoped 样式，不污染外部系统 */
.gochat-widget {
  font-family: 'Noto Sans SC', sans-serif;
  /* ... */
}
</style>
```

### 插件安装方式

```javascript
// 外部系统安装
import GoChat from '@gochat/vue-chat'

app.use(GoChat, {
  apiBaseUrl: 'http://gochat:8080/api/v1',
  authToken: 'gochat_jwt_token'
})

// 在任意页面使用
<template>
  <chat-widget :userId="currentUser.id" :friends="userList" />
</template>
```

### WebSocket 连接管理

外部系统传入 token 后，组件自动连接 WebSocket。组件内部管理心跳和重连，对外通过 props 和 events 通信。

---

## 第五章：配置 12-factor

所有配置通过环境变量覆盖，默认值从配置文件读取。

```yaml
server:
  api_addr: ":8080"
  connect_addr: ":7000"

auth:
  mode: external              # builtin | external
  external_jwks_url: "${AUTH_JWKS_URL}"
  external_jwt_secret: "${AUTH_JWT_SECRET}"

mysql:
  host: "${MYSQL_HOST:localhost}"
  port: ${MYSQL_PORT:3306}
  user: "${MYSQL_USER:gochat}"
  password: "${MYSQL_PASSWORD}"
  database: "${MYSQL_DB:gochat}"

minio:
  endpoint: "${MINIO_ENDPOINT:localhost:9000}"
  access_key_id: "${MINIO_ACCESS_KEY}"
  secret_access_key: "${MINIO_SECRET_KEY}"
  bucket: "${MINIO_BUCKET:gochat}"
```

---

## 第六章：实施计划

### Phase 1: 基础设施

- 添加 `external_user` 表和模型
- 实现 `POST /api/v1/auth/exchange` Token 交换端点
- 实现 `auth.mode` 配置切换（builtin / external）
- `from_id` BIGINT → VARCHAR(64)

### Phase 2: API 版本化

- 注册 `/api/v1/` 路由组
- 添加 `/healthz` 和 `/api/v1/version` 端点
- 兼容旧路由

### Phase 3: 前端组件化

- 提取 `ChatWidget.vue` 核心组件
- 实现 props/events API
- `useChat` composable 封装 WS 连接和消息状态
- 发布 npm 包（可选）

### Phase 4: 文档与示例

- 编写集成文档
- 提供外部系统对接示例
