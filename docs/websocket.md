# WebSocket 协议文档

## 概述

WebSocket 端点: `/ws?token={jwt}`

所有消息采用 JSON 格式，通用结构：

```json
{ "cmd": "命令类型", "seq": "序列号（请求响应匹配）", "data": { ... } }
```

- `cmd` — 命令类型，见下方各事件定义
- `seq` — 客户端生成的序列号（推荐 `Date.now()`），服务端在响应中回传用于匹配
- `data` — 具体数据，按命令类型不同

---

## 目录

1. [客户端→服务端（上行）](#客户端服务端上行)
   - [ping — 心跳](#ping--心跳)
   - [msg — 发送消息](#msg--发送消息)
   - [msg_read — 消息已读](#msg_read--消息已读)
2. [服务端→客户端（下行）](#服务端客户端下行)
   - [pong — 心跳响应](#pong--心跳响应)
   - [msg_reply — 消息发送确认](#msg_reply--消息发送确认)
   - [msg — 消息转发](#msg--消息转发)
   - [msg_read_reply — 已读确认](#msg_read_reply--已读确认)
   - [offline_notify — 离线通知](#offline_notify--离线通知)
   - [error — 错误响应](#error--错误响应)
3. [服务端事件](#服务端事件)
   - [连接建立](#连接建立)
   - [连接断开](#连接断开)
4. [错误码](#错误码)

---

## 客户端→服务端（上行）

### ping — 心跳

客户端每 30 秒发送一次心跳，维持连接。

> **请求**
```json
{ "cmd": "ping", "seq": "" }
```

> **响应**
见服务端 [pong](#pong--心跳响应)

---

### msg — 发送消息

发送聊天消息。私聊和群聊共用此命令。

> **请求**
```json
{
  "cmd": "msg",
  "seq": "1749218312345",
  "data": {
    "from_id": 1,
    "to_id": 2,
    "group_id": 0,
    "type": "text",
    "content": "你好"
  }
}
```

> **参数说明**

| 字段 | 类型 | 说明 |
|------|------|------|
| `from_id` | number | 发送者 ID |
| `to_id` | number | 接收者 ID（群消息填 0） |
| `group_id` | number | 群组 ID（私聊填 0） |
| `type` | string | 消息类型: `text` `image` `file` `voice` `video` `mention` |
| `content` | string | 文字消息存原文；多媒体消息存 FileMeta JSON |

> **约束**
- `to_id` 和 `group_id` 互斥，只能一个非零
- `content` 不能为空

> **响应**
见服务端 [msg_reply](#msg_reply--消息发送确认)

> **转发**
见服务端 [msg — 消息转发](#msg--消息转发)

---

### msg_read — 消息已读

标记消息为已读，清除该发送者的未读计数。

> **请求**
```json
{
  "cmd": "msg_read",
  "seq": "1749218315678",
  "data": { "message_id": "321622086873387008" }
}
```

> **参数说明**

| 字段 | 类型 | 说明 |
|------|------|------|
| `message_id` | string | 对方发送的最后一条消息 ID（Snowflake），服务端据此查出发送者并清除未读 |

> **处理逻辑**
1. 根据 `message_id` 查询消息记录，获取 `from_id`（发送者）
2. 清除当前用户对该发送者的未读计数：`UPDATE unread_count SET count=0 WHERE user_id=? AND from_id=? AND group_id=?`
3. 返回确认

> **响应**
见服务端 [msg_read_reply](#msg_read_reply--已读确认)

---

## 服务端→客户端（下行）

### pong — 心跳响应

> **响应**
```json
{ "cmd": "pong", "seq": "", "data": null }
```

---

### msg_reply — 消息发送确认

服务端收到 `msg` 后保存成功，向**发送者**返回确认，包含真实消息 ID（Snowflake）。

> **响应**
```json
{
  "cmd": "msg_reply",
  "seq": "1749218312345",
  "data": { "status": "success", "message_id": "321622086873387008" }
}
```

> **注意**
`seq` 值与发送请求的 `seq` 一致，用于客户端匹配确认和重试。

---

### msg — 消息转发

当消息的接收者在线时，服务端将消息转发给接收者。

> **转发**
```json
{
  "cmd": "msg",
  "seq": "",
  "data": {
    "from_id": 1,
    "to_id": 2,
    "group_id": 0,
    "type": "text",
    "content": "你好"
  }
}
```

> **注意**
- 转发消息的 `seq` 为空
- 群聊消息会广播给所有在线群成员（排除发送者）

---

### msg_read_reply — 已读确认

`msg_read` 处理成功后的确认响应。

> **响应**
```json
{
  "cmd": "msg_read_reply",
  "seq": "1749218315678",
  "data": { "status": "success", "message_id": "321622086873387008" }
}
```

---

### offline_notify — 离线通知

用户建立连接后，服务端推送离线期间的未读消息汇总。

> **推送**
```json
{
  "cmd": "offline_notify",
  "seq": "",
  "data": {
    "user_id": "4",
    "unread_list": [
      { "from_id": 5, "group_id": 0, "count": 3 },
      { "from_id": 0, "group_id": 1, "count": 7 }
    ]
  }
}
```

> **参数说明**

| 字段 | 类型 | 说明 |
|------|------|------|
| `user_id` | string | 当前用户 ID |
| `unread_list` | array | 未读汇总列表 |
| `unread_list[].from_id` | number | 发送者 ID（群消息为 0） |
| `unread_list[].group_id` | number | 群组 ID（私聊为 0） |
| `unread_list[].count` | number | 未读消息数量 |

---

### error — 错误响应

当处理失败时，服务端返回错误。

> **格式**
```json
{
  "cmd": "error",
  "seq": "1749218312345",
  "data": { "code": 49001, "message": "消息格式无效" }
}
```

> **通用错误码**

| code | 说明 |
|------|------|
| 49001 | 消息格式无效 |
| 49002 | 消息发送失败 |
| 49004 | 消息不存在 |
| 49005 | 用户不在线 |
| 49006 | 发送队列已满 |

完整错误码见 [错误码表](#错误码)。

---

## 服务端事件

### 连接建立

1. 客户端通过 `/ws?token={jwt}` 建立 WebSocket 连接
2. 服务端验证 JWT Token，获取 `userID`
3. 如果该用户已有在线连接，踢出旧连接（单点登录）
4. 触发 `OnConnect` 回调：
   - 记录上线日志
   - 如果有离线期间的未读消息，推送 [offline_notify](#offline_notify--离线通知)
5. 启动读取/写入协程

### 连接断开

1. 客户端正常关闭或异常断开
2. 从连接池移除用户
3. 关闭发送队列
4. 触发 `OnDisconnect` 回调：
   - 记录下线日志

---

## 错误码

### WebSocket 错误码 490xx

| code | 名称 | 说明 |
|------|------|------|
| 49001 | CodeWsInvalidMsg | 消息格式无效 |
| 49002 | CodeWsSendFailed | 消息发送失败 |
| 49004 | CodeWsNotFound | 消息不存在 |
| 49005 | CodeWsOffline | 用户不在线 |
| 49006 | CodeWsQueueFull | 发送队列已满 |

### 完整错误码

| 范围 | 模块 |
|------|------|
| 400xx | 请求参数 |
| 401xx | 认证授权 |
| 410xx | 用户模块 |
| 420xx | 消息模块 |
| 430xx | 文件上传 |
| 440xx | 好友模块 |
| 450xx | 群组模块 |
| 490xx | WebSocket 通信 |

---

## 完整通信时序

### 私聊消息流程

```
发送者 A                  服务端                  接收者 B
  │                         │                        │
  │── msg ────────────────► │                        │
  │                         ├─ 保存消息到 DB          │
  │                         ├─ IncrUnread(B, A, 0)   │
  │                         │                        │
  │◄── msg_reply ────────── │                        │
  │                         │                        │
  │                         ├─ [B在线] ── msg ──────►│
  │                         │                        │
  │                         │   B打开聊天后:          │
  │                         │◄── msg_read ────────── │
  │                         ├─ GetByID(message_id)   │
  │                         ├─ ClearUnread(B, A, 0)  │
  │                         ├──► msg_read_reply ────►│
```

### 离线消息流程

```
B 建立连接                  服务端
  │                         │
  │── ws?token=xxx ────────►│
  │                         ├─ 验证 Token
  │                         ├─ HandleConnect
  │                         │  └─ GetOfflineNotify(B)
  │                         │     └─ 查询 unread_count
  │◄── offline_notify ───── │
  │    {unread_list}        │
  │                         │
  │── msg_read ────────────►│
  │                         ├─ ClearUnread(B, A, 0)
```
