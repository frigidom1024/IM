# GoChat 领域概念与接口调用指南

本文面向前端、后端和第三方系统的接入开发者，说明业务对象之间的关系、权限规则和接口调用顺序。术语定义见 [CONTEXT.md](../CONTEXT.md)，部署与 SDK 接入见 [集成指南](integration.md)，完整字段与参数限制以 [HTTP OpenAPI](api/openapi.json) 和 [WebSocket 协议](websocket.md) 为准。

## 1. 先分清业务对象

| 概念 | 代表什么 | 调用接口时的含义 |
| --- | --- | --- |
| GoChat 用户 | 本地聊天身份 | 认证决定当前操作者，外部用户 ID 需先映射 |
| 好友关系 | 我将谁加入联系人 | 单向；好友列表不等于会话列表 |
| 好友通知 | 谁将我加入联系人 | 通知已读与聊天已读分别处理 |
| 群组 | 共享聊天范围 | 群有成员、容量、公告和群主 |
| 群成员资格 | 某用户是否在群中及其角色 | 决定群历史访问、发送和管理权限 |
| 入群申请 | 尚未完成的加入请求 | `pending` 不代表已经是成员 |
| 会话 | 当前用户在私信或群聊中的个人视图 | 管理列表、未读、阅读状态和个人偏好 |
| 消息 | 一次发送的内容记录 | 有独立消息 ID；同一消息可出现在多个用户的会话中 |
| 媒体资产 | 上传完成的文件或媒体 | 上传成功后还需发送引用它的消息 |
| 上传会话 | 一份文件的上传进度 | 与聊天会话无关 |

打开聊天页时查询会话列表，展示联系人时查询好友列表，展示群时查询群列表。添加好友、创建空群或加入空群不会自动产生聊天会话。

一次新消息会更新参与者的会话摘要：发送者的未读不增加，私信接收者或其他当前群成员的未读增加。消息和会话更新成功提交后才返回发送确认。

## 2. 身份、ID 和响应约定

### 2.1 当前用户来自认证

受保护 HTTP 请求携带：

```http
Authorization: Bearer <GoChat JWT>
```

内置模式调用 `POST /api/auth/register`、`POST /api/auth/login`；外部模式用外部系统 token 调用 `POST /api/auth`，取得 GoChat token 和本地 `user.id`。两种模式按配置启用，不能混用路由。外部系统返回的用户 ID 只用于身份映射，不能直接作为 `friend_id`、群成员 `user_id` 或私信目标。

WebSocket 握手也使用 GoChat token。后端客户端可用 `Authorization` 头；浏览器可连接 `/ws?token=<GoChat JWT>`。同一用户的新连接会替换旧连接，接入方应协调连接所有权。

### 2.2 会话 ID 是当前用户视角

设 A 的 GoChat ID 为 1，B 为 2，群 G 为 9：

| 当前用户 | 业务范围 | `conversation_id` / HTTP `{scope}` |
| --- | --- | --- |
| A | 与 B 私信 | `private:2` |
| B | 与 A 私信 | `private:1` |
| A 或 B | 群 G | `group:9` |

私信双方使用不同的会话 ID。会话 ID 必须结合当前认证身份理解，不能当作双方共享的全局 ID。`scope` 只接受 `private:<正整数>`、`group:<正整数>`，不接受 `private:02`、0、负数或自己与自己的私信。

会话的数据库身份是 `(from_id,to_id,group_id)`，其中 `from_id` 是会话拥有者；消息里的 `from_id` 则是作者。调用方不需要提交数据库组合键，HTTP 使用 scope、WS 使用 `conversation_id`，拥有者与作者从认证身份确定。

### 2.3 消息 ID 与序列号用途不同

| 标识 | 来源 | 用途 |
| --- | --- | --- |
| `conversation_id` | 当前用户的业务范围 | 定位个人会话 |
| 消息 ID | 服务端生成 | 历史与实时消息去重、关联发送确认 |
| `seq` | 客户端生成 | 关联本次发送与 ACK、幂等重试 |
| `applicationId` | 入群申请返回值 | 审核具体申请；不是申请人的用户 ID |
| `fileId` / 上传 ID | 上传接口返回值 | 下载文件或继续上传；不是消息 ID |

消息 ID 可超过 JavaScript 安全整数范围。WS 下行 `id`、`sender_id` 是十进制字符串；ACK 优先使用 `message_id_string`；HTTP 历史优先使用 `id_string`。会话摘要中的 `last_message.id` 和未读项的 `latest_message_id` 也是字符串。用户、群和申请 ID 当前仍可能是 JSON 整数，调用方需正确处理 int64，不能从已经舍入的 Number 恢复原始值。

HTTP 历史保留 `from_id`、`to_id`、`group_id`、`msg_type`，与 WS 的 `conversation_id`、`sender_id`、`type` 不是同一套字段。历史作者使用 `from_id`，兼容字段 `sender_id` 可能为 0。

### 2.4 HTTP 成功、错误和分页

JSON 接口成功时 `code: 0`，业务结果放在 `data`：

```json
{"code":0,"message":"ok","data":null}
```

无数据的写操作也可能成功返回 `data: null`。错误返回非零业务码、说明和 `data: null`；同时检查 HTTP 状态与业务码。文件下载返回二进制，不按 JSON 封装解析。

支持分页的接口使用 `limit` 和条目偏移 `offset`，默认 `limit=50`、`offset=0`，`limit` 范围 1–100。历史按 `created_at DESC, id DESC` 返回，UI 可重新排序为时间正序。加载下一页时增加条目 offset，不能把消息 ID 当作 offset。实时新增会移动偏移分页的位置，合并 WS 与分页结果时按精确消息 ID 去重。

## 3. 一个最小私信调用流程

下面假定 A、B 已分别认证，`BASE` 是 GoChat HTTP 地址，`TOKEN_A`、`TOKEN_B` 是各自的 GoChat token，B 的本地用户 ID 为 2：

1. A 添加 B：`POST /api/friend/add`，请求体 `{"friend_id":2}`。
2. B 可查询好友通知并决定是否反向添加 A。A 单向添加后最多可先发一次文本。
3. A 显式打开 `private:2`，查询会话摘要和历史，确认当前发送权限。
4. A 通过 WS 发送，使用非空 `seq`，等待对应 `msg_reply` 或 `error`。
5. B 收到实时消息或主动查询历史；实际打开并处理该会话后，通过 HTTP 标记整个 `private:1` 已读。

A 的 HTTP 示例：

```bash
curl -X POST "$BASE/api/friend/add" \
  -H "Authorization: Bearer $TOKEN_A" \
  -H 'Content-Type: application/json' -d '{"friend_id":2}'

curl -X POST "$BASE/api/conversation/private:2/open" \
  -H "Authorization: Bearer $TOKEN_A"

curl "$BASE/api/conversation?limit=50&offset=0" \
  -H "Authorization: Bearer $TOKEN_A"

curl "$BASE/api/conversation/private:2/messages?limit=50&offset=0" \
  -H "Authorization: Bearer $TOKEN_A"
```

示例只演示流程；请求成功并不意味着后续步骤无需检查权限或错误。发送帧与确认见第 6 节。

## 4. 好友关系与私信发送权限

### 4.1 添加好友是单向关系

| 操作 | 接口 | 结果 |
| --- | --- | --- |
| 我添加对方 | `POST /api/friend/add`，`{friend_id, remark?}` | 创建“我 → 对方”关系，不自动创建反向关系 |
| 查看我添加的人 | `GET /api/friend/list` | 每项 `friend_id` 是对方；`added_by_peer` 表示对方是否也添加了我 |
| 查看谁添加了我 | `GET /api/friend/incoming` | 每项 `user_id` 是添加者，带独立 `unread` |
| 标记好友通知已读 | `POST /api/friend/{friendId}/notice/read` | 路径是添加者用户 ID，不是通知记录 ID |
| 我删除对方 | `DELETE /api/friend/{friendId}` | 删除自己的单向关系，不删除聊天历史 |

好友通知是来向好友关系的当前视图，添加者删除关系后，该通知会从接收者列表消失。查看或标记通知已读不会自动互加好友；要互加，接收者也调用 `POST /api/friend/add`。

### 4.2 以会话返回的发送策略为依据

会话摘要的 `send_policy`、`intro_remaining`、`added_by_me`、`added_by_peer` 描述当前用户对该私信的发送能力：

| 我的关系状态 | `send_policy` | 可以发送什么 |
| --- | --- | --- |
| 我未添加对方，包括只有对方添加了我 | `unavailable` | 不可发送 |
| 我添加对方，对方未添加我，首次额度未使用 | `text_once` | 一条 `text`，不能发送媒体或 `mention` |
| 我添加对方，对方未添加我，首次额度已使用 | `unavailable` | 等待互加好友 |
| 双方互加 | `unrestricted` | 正常发送支持的消息类型 |

首次额度按发送方向记录。删除再添加、关系变化或撤回标记不会重置已经使用的额度；重试同一条已成功保存的消息也不会再次扣减额度。权限字段是展示依据，服务端在发送时仍会校验当前状态。

已有历史可以保留并查询。存在会话、能打开会话或能查询历史，都不代表有发送权限。

## 5. 会话列表、历史、已读与偏好

| 接口 | 行为 | 调用时机 |
| --- | --- | --- |
| `GET /api/conversation` | 查询当前用户会话摘要，置顶优先 | 会话页、登录后、重连后 |
| `POST /api/conversation/{scope}/open` | 显式建立个人视图，不发送消息 | 从联系人或群列表打开聊天 |
| `GET /api/conversation/{scope}/messages` | 查询授权历史，不创建视图、不标记已读 | 展示历史、补拉断线消息 |
| `GET /api/conversation/unread` | 返回 `count`、兼容 `total_unread_count` 和有未读的 `unread_list` | 总角标、启动与恢复同步 |
| `POST /api/conversation/{scope}/read` | 清除整个会话当前未读，并设置 `read_at` | 确认用户实际阅读该会话后 |
| `PATCH /api/conversation/{scope}` | 修改自己的置顶、免打扰或草稿，可建立空视图 | 偏好或草稿变化时 |

空会话的 `last_message`、`last_message_at` 可以为 null。发送/接收新消息会建立会话；空群的创建或加入不会，加入有历史的群可初始化已有消息摘要，历史不自动计作新成员未读。群历史查询依据当前成员资格，不按入群时间裁剪。

标记已读前应先存在会话，可通过 `open` 或成功发送/接收建立；查询历史本身不会建立它，直接对不存在的视图标记已读会失败。

读取整个会话不带消息 ID、游标或请求体：

```http
POST /api/conversation/private:1/read
Authorization: Bearer <B 的 GoChat JWT>
```

成功返回当前 scope、`unread_count: 0` 和 `read_at`。这不是“截至某条消息已读”：服务端操作时已经提交到该会话的未读会清零，之后提交的新消息继续产生未读。调用方应在历史成功加载、会话仍然可见时决定标记；后台补拉或隐藏页面不应自动清零。请求失败时保留本地未读显示，成功后再更新。

个人偏好示例：

```http
PATCH /api/conversation/group:9
Authorization: Bearer <GoChat JWT>
Content-Type: application/json

{"pinned":true,"muted":true,"draft":"待发送的内容"}
```

字段可分别提交，至少提供一个；草稿上限 4096 UTF-8 字节。`false` 表示取消，空字符串表示清空草稿，省略表示不改。个人 `muted` 是免打扰偏好，不阻止消息保存、投递或未读统计；群成员 `muted_until` 才控制能否在群里发言。

当前没有 WS 已读命令、单条消息已读接口或群成员消息回执。不要调用旧 `/api/message/read`、`/api/message/unread` 或旧私信/群历史接口。

## 6. 发送、确认与重试

### 6.1 通过 WS 发送消息

当前公开协议通过 WS `msg` 发送；HTTP 会话接口负责查询和个人状态。A 发给 B：

```json
{
  "cmd":"msg",
  "seq":"a-message-001",
  "data":{"conversation_id":"private:2","type":"text","content":"你好"}
}
```

客户端只提交会话范围、类型和内容，不提交三个目标 ID 或作者身份。支持 `text`、`image`、`file`、`voice`、`video`、`mention`；内容必须非空，媒体内容为 JSON 编码的 FileMeta 字符串。

新消息持久保存后，A 收到确认：

```json
{
  "cmd":"msg_reply",
  "seq":"a-message-001",
  "data":{
    "status":"success",
    "conversation_id":"private:2",
    "message_id":123,
    "message_id_string":"123",
    "duplicate":false
  }
}
```

在线 B 的下行消息使用 B 的会话视角：

```json
{
  "cmd":"msg",
  "seq":"a-message-001",
  "data":{
    "id":"123",
    "conversation_id":"private:1",
    "sender_id":"1",
    "type":"text",
    "content":"你好",
    "created_at":"2026-10-14T08:00:00Z"
  }
}
```

群消息双方都使用 `group:9`。作者自己的连接通过 ACK 获得消息 ID，其他在线接收者获得 `msg`。ACK 只说明消息与会话状态保存成功，不说明对方在线、已收到或已读；离线接收者通过 HTTP 查询恢复。

业务失败使用相同 `seq` 的 `error` 命令，`data` 为 `{code,message}`。连接或解码异常与服务端业务失败分别处理。

### 6.2 超时不等于发送失败

确认可能在断线期间丢失。每条新消息生成一个非空、至多 128 字节的 `seq`；重试保留同一发送者、同一 `seq`、同一会话、类型和原始内容。相同 `seq` 在不同会话也不能复用。

成功保存后的相同重试返回原消息 ID、`duplicate: true`，不新增消息、不再次增加未读，也不重新投递给接收者。复用 `seq` 修改范围或内容会报错；空 `seq` 不启用幂等。

客户端将临时消息与 ACK 关联后，按精确消息 ID 与 WS/HTTP 历史合并。Go SDK `SendChat` 每次生成新序列号；重试用 `Send` 提交原始完整帧。`Receive()` 是持续消息通道，`Errors()` 是接收异常通道，服务端 `error` 帧仍属于消息通道。具体接入示例见 [Go SDK](../sdk/README.md)。

## 7. 群、成员与入群申请

### 7.1 申请流程

| 步骤 | 调用者 | 接口与关键数据 |
| --- | --- | --- |
| 创建群 | 已认证用户 | `POST /api/group/create`，`{name, max_members?, avatar?}`；创建者成为群主 |
| 搜索群 | 已认证用户 | `GET /api/group/search?q=项目`，搜索结果不授予成员资格 |
| 申请加入 | 非成员 | `POST /api/group/{groupId}/join`；返回申请 ID 与 `status: pending` |
| 查看自己的申请 | 申请人 | `GET /api/group/applications/mine` |
| 查看群申请 | 当前群主 | `GET /api/group/{groupId}/applications` |
| 审核 | 当前群主 | `POST /api/group/{groupId}/applications/{applicationId}/review`，`{decision, reason?}` |
| 确认成员资格 | 申请人 | 查询 `GET /api/group/list`，再打开群会话、查询群历史 |

申请状态为 `pending`（待处理）、`approved`（通过）、`rejected`（拒绝）、`closed`（群已解散而关闭）。`decision` 使用 `approve` 或 `reject`，不是状态字段的过去式。存在 pending 时重复申请返回原申请；被拒绝后可以创建新申请。群满时批准返回 409，申请继续 pending。

例如群主审核申请 15 加入群 9：

```http
POST /api/group/9/applications/15/review
Authorization: Bearer <当前群主 JWT>
Content-Type: application/json

{"decision":"approve","reason":"欢迎加入"}
```

群主/管理员可以直接 `POST /api/group/{groupId}/invite`，提交 `{"user_id":2}`，成功后直接成为成员；该用户已有 pending 申请时也会同步为 approved。邀请流程与主动申请不同。

### 7.2 角色与管理权限

角色为普通成员 0、管理员 1、群主 2。当前审核申请和查看群申请列表的权限只属于群主，不能仅凭“管理员”展示审核能力。

| 操作 | 当前权限规则 | 接口 |
| --- | --- | --- |
| 查群成员、群历史、群文件 | 当前成员 | `/members`、会话 `/messages`、群 `/files` |
| 发群消息 | 当前成员且不在禁言期 | WS `msg`，scope 为 `group:<群ID>` |
| 邀请成员、改公告 | 群主或管理员 | `POST /invite`、`PUT /announcement` |
| 改自己的群昵称 | 当前成员 | `PATCH /members/{userId}` |
| 改他人昵称、禁言或移除他人 | 群主/管理员，目标角色必须更低 | `PATCH /members/{userId}`、`DELETE /members/{userId}` |
| 设管理员或普通成员 | 群主，不能直接修改群主角色 | `PATCH /members/{userId}`，`role: 1` 或 `0` |
| 转让、解散 | 当前群主 | `POST /transfer`、`DELETE /api/group/{groupId}` |
| 主动退出 | 普通成员或管理员 | `DELETE /leave`；群主先转让，或解散群 |

表中的短路径位于 `/api/group/{groupId}` 下。转让请求体为 `{"user_id":<现有成员ID>}`；转让后原群主变普通成员，申请审核权随群主变更。

退出、被移除或群解散后，该群从个人会话列表与未读汇总中隐藏，当前成员资格不再允许查询群历史或发群消息。历史内容与个人偏好保留；重新加入后的实际状态以接口结果为准。不要仅删除 UI 项后继续沿用旧权限。

群会话摘要的 `send_policy` 当前为 `unrestricted`，它不是完整群权限判定；禁言和成员变动仍需结合群成员状态，并处理发送时的错误。

## 8. 上传文件与发送媒体消息

上传文件和发送消息是两个步骤。上传成功返回 FileMeta 不会产生聊天消息。群文件列表包含声明归属于该群的未过期资产，以及通过未撤回群消息分享的未过期资产；指定 `group_id` 上传的资产可以在发送消息前就出现在群文件列表。

简单上传：

```bash
curl -X POST "$BASE/api/upload" \
  -H "Authorization: Bearer $TOKEN_A" \
  -F 'type=file' -F 'file=@report.pdf'
```

群上传可增加 `group_id`。响应 `data` 包含 `url`、`name`、`size`、`mime` 等信息。发送时使用上传返回的元数据，编码为字符串内容，例如：

```js
wsClient.sendChat('private:2', JSON.stringify(fileMeta), 'file')
```

媒体类型需满足当前私信或群发送权限，单向好友的首次额度只允许文本。FileMeta 的 `/api/file/{fileId}/download` 和缩略图地址需要 GoChat token，不是公开或预签名 URL。浏览器通过带认证的请求获取 Blob 后展示，不能假设直接放进 `<img src>` 就能完成认证。

资产拥有者、资产所属群的当前成员，以及通过有效消息分享取得权限的接收者可按规则下载。私信接收者或其他通过消息分享取得权限的人，需要等消息成功保存后才能访问；仅把 URL 交给他人不会授予权限。过期资产返回 410，权限不足返回 403。

需要断点上传时：

| 顺序 | 接口 | 注意事项 |
| --- | --- | --- |
| 1 | `POST /api/upload/session` | 声明 `name,type,mime,size`，可指定群和资产过期时间 |
| 2 | `GET /api/upload/session/{uploadId}` | 获取服务端已提交的 `offset` |
| 3 | `PATCH /api/upload/session/{uploadId}` | 二进制块，`Upload-Offset` 必须等于当前偏移，每块最多 8 MiB |
| 4 | `POST /api/upload/session/{uploadId}/complete` | 全部字节上传后处理并取得 FileMeta |
| 取消 | `DELETE /api/upload/session/{uploadId}` | 中止上传 |

偏移冲突返回 409，重新查询进度再继续，不能盲目追加同一块。上传会话 24 小时后过期；最终资产是否过期由 `expires_hours` 决定。尺寸、MIME、媒体时长等限制见 OpenAPI 和 SDK 文档。

## 9. 登录、重连与通知恢复

建立 WS 连接不会推送离线通知。登录成功、重连成功和页面重新可见时，按业务需要主动查询：

| 页面数据 | 接口 | 状态归属 |
| --- | --- | --- |
| 会话列表与未读 | `GET /api/conversation`、`GET /api/conversation/unread` | 当前用户的聊天会话 |
| 当前会话补拉 | `GET /api/conversation/{scope}/messages` | 消息历史；展示后再决定是否标记已读 |
| 好友与好友通知 | `GET /api/friend/list`、`GET /api/friend/incoming` | 单向关系与通知阅读状态 |
| 当前所属群 | `GET /api/group/list` | 成员资格、角色与待处理数量 |
| 我的申请结果 | `GET /api/group/applications/mine` | 当前用户的入群申请 |
| 待处理群申请 | `GET /api/group/{groupId}/applications` | 当前群主处理的申请 |

在线 `friend_changed`、`group_changed` 仅通知客户端刷新，可能因断线丢失；不能以是否收到事件判断持久状态。好友通知已读、会话已读和入群审核是三种独立操作。群申请没有另一个“标记通知已读”接口，按状态展示和处理。

自行实现客户端时，先订阅实时消息，再主动查询并按消息 ID 合并，避免“查询完成到订阅建立”的间隙丢消息。恢复未确认的发送时按原 `seq` 重试；恢复当前历史后才决定清零未读。切换用户时关闭旧连接、清除旧用户缓存，再按新身份查询。

## 10. 常见错误与定位

| 现象 | 应检查什么 |
| --- | --- |
| 认证 401 | 区分外部 token 与 GoChat token；外部模式检查用户接口是否可达、返回直接 JSON |
| 对方添加我后仍不能回复 | 我是否反向添加；来向通知不是双向好友关系 |
| 首次文本之后不能发媒体 | 是否已互相添加；检查 `send_policy`，删除再加不会重置额度 |
| 能看到历史但发送失败 | 私信发送权限、群成员资格和禁言状态 |
| 申请入群返回成功，但群历史 403 | `pending` 未被批准，不是成员 |
| 管理员查询申请或审核 403 | 当前实现仅群主拥有申请审核权限 |
| 标记已读 404 | 会话视图是否建立；先 `open`，而非只查询历史 |
| 重试后 `seq` 冲突 | 是否复用了其他会话或内容的发送序列 |
| 收到重复消息或 ID 不一致 | 是否用了精确字符串 ID；HTTP 与 WS 字段是否正确适配 |
| 下载链接 403/401 | 请求是否携带 token，当前身份是否具有资产访问权限 |
| 重连后通知没变化 | 是否主动查询 HTTP 状态；不是等待离线推送 |

HTTP 权限错误通常为 403，参数错误为 400，容量或上传偏移冲突为 409，过期资产为 410；WS 业务错误使用 `error` 帧中的业务码。具体错误码见 [HTTP 运维契约](operations/runtime.md) 和 [WS 协议](websocket.md)。

## 11. SDK 方法对照

| 业务 | 前端核心库 | Go SDK |
| --- | --- | --- |
| 外部认证 | `useUserStore().externalAuth()` | `ExternalAuth()` 后使用 `WithToken()` |
| 会话列表 | `useConversationStore().loadConversations()` | `ListConversations()` |
| 打开与历史 | `useChatStore().openChat()` | `OpenConversation()`、`ListPrivateMessagesPage()` / `ListGroupMessagesPage()` |
| 未读与整会话已读 | `conversationApi.getUnreadCount()` / `markRead()` | `GetUnreadSummary()`、`MarkConversationRead()` |
| 好友通知 | `friendApi.getIncoming()` / `readNotice()` | `ListFriendNotices()`、`ReadFriendNotice()` |
| 申请入群 | `groupApi.join()` | `ApplyToGroup()`，直接获得申请结果 |
| 发送与接收 | `wsClient.sendChat()` / `on()` | `SendChat()` / `Receive()`，接收异常另读 `Errors()` |

前端高层 `openChat` 还会在当前会话可见、历史成功加载后标记已读；Go SDK 调用方自行决定阅读时机。旧 Go SDK `JoinGroup()` 对 pending 返回 `ErrGroupApprovalPending`，它表示等待审核，应通过申请结果与群列表继续确认，不能当作已入群。

SDK 的部署、引入和完整示例见 [集成指南](integration.md)。接口新增或行为变化时，同步更新本指南、OpenAPI、WS 协议和相关 SDK 文档。
