# GoChat 外部系统集成指南

## 概述

GoChat 可以作为微服务集成到其他系统中。外部系统只需提供一个「获取用户信息」的 API，即可与 GoChat 打通认证。

---

## 工作方式

### 架构图

```
外部系统(你的项目)           GoChat 微服务
┌───────────────────┐     ┌──────────────────────────┐
│ 外部前端页面        │     │ Gin API                  │
│ ┌───────────────┐ │     │ ├─ POST /api/auth         │
│ │ ChatWidget    │ │──REST│ ├─ GET  /api/message/*    │
│ │ (嵌入的聊天组件)│ │──WS─►│ ├─ POST /api/upload      │
│ └───────────────┘ │     │ ├─ WS   /ws               │
│                   │     │ └─ GET  /healthz           │
│ 外部用户服务       │     │                          │
│ GET /api/user/info │◄────│ (GoChat 启动时调用)       │
└───────────────────┘     └──────────────────────────┘
```

### 认证时序

```
前端(已有外部JWT)          GoChat                      外部系统
  │                         │                            │
  │ 用户在外部系统已登录     │                            │
  │ 外部JWT 已存储在本地     │                            │
  │                         │                            │
  │ ChatWidget 初始化        │                            │
  │    ↓                    │                            │
  │── POST /api/auth ──────►│                            │
  │   Authorization:        │                            │
  │   Bearer <外部JWT>      │                            │
  │                         │                            │
  │                         ├── GET /api/user/info ────► │
  │                         │   Authorization: Bearer xx │
  │                         │   (原样转发前端传的token)   │
  │                         │                            │
  │                         │◄── 200 OK ──────────────── │
  │                         │   {                        │
  │                         │     "id": "42",            │
  │                         │     "name": "张三",         │
  │                         │     "avatar": ""           │
  │                         │   }                        │
  │                         │                            │
  │                         ├─ 查找/创建用户              │
  │                         │  username = "ext_42"       │
  │                         │  nickname = "张三"         │
  │                         │                            │
  │                         ├─ 生成 gochat JWT            │
  │                         │                            │
  │◄── 200 OK ─────────────┤                            │
  │   {                    │                            │
  │     "token": "xxx",     │                            │
  │     "user": {"id":1,    │                            │
  │              "name":"张三"}                           │
  │   }                    │                            │
  │                         │                            │
  │ ChatWidget 自动处理:     │                            │
  │ 1. 存 token 到 storage  │                            │
  │ 2. 连 WebSocket         │                            │
  │ 3. 加载好友列表          │                            │
```

---

## 第一部分：外部系统需要做的事

### 1. 实现一个用户信息 API

提供一个 HTTP GET 接口，供 GoChat 调用。GoChat 在接收前端认证请求时会原样转发 `Authorization` 头到这个接口。

> **要求：** 这个 API 必须在 GoChat 可以访问到的地址上（同网络、或公网可达）。

```python
# Python / FastAPI 示例
from fastapi import FastAPI, Header, HTTPException

app = FastAPI()

@app.get("/api/user/info")
async def get_user_info(authorization: str = Header(None)):
    # 1. 验证外部系统的 JWT
    if not authorization:
        raise HTTPException(401, "Missing token")

    # 2. 按外部系统的认证逻辑验证 token
    user = your_auth_logic(authorization)

    # 3. 返回以下格式
    return {
        "id": str(user.id),      # 必填：用户唯一标识
        "name": user.nickname,    # 必填：显示名称
        "avatar": user.avatar or ""  # 可选：头像 URL
    }
```

### 2. 在页面中嵌入 ChatWidget

#### npm 方式（推荐）

```bash
npm install @gochat/vue-chat
```

```vue
<template>
  <div style="width: 100%; height: 600px;">
    <chat-widget
      :token="userToken"
      :contacts="friendList"
    />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ChatWidget } from '@gochat/vue-chat'

// 用户在外部系统的 JWT
const userToken = ref(localStorage.getItem('external_jwt'))

// 联系人列表（可选）
const friendList = ref([
  { id: 1, name: '张三' },
  { id: 2, name: '李四' }
])
</script>
```

#### CDN 方式

```html
<div id="gochat"></div>
<script src="https://cdn.example.com/gochat-chat-widget.umd.js"></script>
<script>
  GoChat.render('#gochat', {
    token: '外部JWT',
    contacts: [{ id: 1, name: '张三' }]
  })
</script>
```

### 3. 新用户注册时

当外部系统有新用户注册时，如果想让该用户立刻能用 GoChat，**不需要额外操作**。用户第一次打开聊天组件时，GoChat 会自动调用外部 API 获取信息并创建本地记录。

---

## 第二部分：GoChat 端配置

### 开发环境

编辑 `internal/config/config-dev.yaml`：

```yaml
auth:
  mode: external                                         # 切换到外部认证
  external_user_api: "http://host.docker.internal:8000/api/user/info"
```

- `external_user_api`：GoChat 调用外部 API 的完整 URL
- `host.docker.internal`：Docker 容器内访问宿主机的方式（如果外部系统运行在宿主机上）

### Docker 部署

通过环境变量覆盖：

```bash
docker run -e AUTH_MODE=external \
  -e AUTH_EXTERNAL_API=http://host:8000/api/user/info \
  gochat-server:latest
```

---

## 第三部分：用户映射规则

GoChat 不修改任何表结构。用户映射逻辑如下：

| 外部 API 返回 | 对应 GoChat user 表 |
|---------------|-------------------|
| `id: "42"` | `username = "ext_42"` |

- `username` 加上 `ext_` 前缀避免与 GoChat 自己注册的用户冲突
- `nickname` 和 `avatar` 直接从外部 API 同步
- 每次认证时会更新用户信息，保持同步
- `user.id`（自增主键）用作消息、好友等内部关联

---

## 第四部分：完整集成示例

### 场景：外部项目集成 GoChat 聊天

```
External Demo (Go SDK)              GoChat
  │                                      │
  │ 1. 启动 GoChat Docker 镜像            │
  │    docker compose up -d              │
  │                                      │
  │ 2. 配置 auth.mode = external         │
  │    external_user_api =               │
  │    "http://external-app:8090/api/user/info"│
  │                                      │
  │ 3. 实现 /api/user/info 接口           │
  │    返回登录用户的 id 和 name          │
  │                                      │
  │ 4. 前端页面嵌入 ChatWidget            │
  │    传入用户的外部 JWT             │
  │                                      │
  │ 5. ✅  集成完成                       │
  │    外部项目用户可以直接聊天            │
```

---

## 第五部分：可运行的集成示例

仓库提供了一个不依赖第三方运行时依赖的最小外部应用，可直接验证外部认证、好友接口、WebSocket 私聊和消息持久化。后端系统也可以直接使用仓库提供的 Go SDK：

- [examples/external-integration-demo/README.md](../examples/external-integration-demo/README.md)
- [后端 Go SDK](../sdk/README.md)

## 第六部分：常见问题

### Q: 外部 API 返回什么格式？

```json
{
  "id": "42",
  "name": "张三",
  "avatar": ""
}
```

- `id` — **必填**，字符串类型
- `name` — **必填**，显示名称
- `avatar` — 可选，头像 URL

### Q: GoChat 修改了用户表吗？

**没有。** 所有改动都是代码层面，不涉及表结构变更。GoChat 用 `ext_` 前缀的 `username` 来标识外部用户。

### Q: 用户 ID 冲突怎么办？

通过 `ext_` 前缀隔离。外部系统的 `id: "42"` 对应 GoChat 的 `username: "ext_42"`，与 `username: "test"` 这种注册用户不会冲突。

### Q: 用户改名了怎么办？

每次认证都会同步外部 API 返回的 `name`，自动更新 `nickname` 字段。

### Q: GoChat 还要继续维护内置注册/登录吗？

要。`auth.mode: builtin` 是默认模式，内置的注册/登录功能完全保留，用于独立部署场景。
