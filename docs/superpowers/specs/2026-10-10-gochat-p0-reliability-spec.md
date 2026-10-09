# GoChat P0 可靠集成规格

> 日期：2026-10-10  
> 状态：Draft / 已按推荐方案确认  
> 目标：把 GoChat 从“可以演示和集成”提升为“外部应用可以依赖的单实例聊天基础服务”。

## Problem Statement

GoChat 当前已经具备 HTTP API、WebSocket、内置认证、外部认证、前端 SDK、后端 Go SDK，以及外部应用 E2E 演示。但要作为其它应用的聊天基础设施使用，还存在几类风险：

- 应用启动和关闭的生命周期不完整，依赖失败和退出行为不够确定；
- 外部认证、JWT、CORS、WebSocket 鉴权等安全约束还没有形成生产级契约；
- WebSocket 已有 ACK，但消息缺少明确的幂等语义，网络重试可能导致重复消息；
- 离线消息、未读数、已读处理还没有形成完整、可验证的交互闭环；
- SDK 已经能够调用主要接口，但还需要稳定的错误契约、版本化文档、重连行为和集成测试；
- 当前测试集存在历史接口不一致，不能用 `go test ./...` 作为可靠的发布门槛；
- Redis、etcd 等配置项与运行时实际使用情况没有清晰的 required/optional 语义。

P0 不解决所有产品功能，而是先解决“外部应用接入后，服务是否可靠、行为是否可预期、失败是否可恢复”的问题。

## Solution

P0 将 GoChat 定义为**单实例部署下、至少一次投递语义、向后兼容的聊天基础服务**。

外部应用应能够完成以下完整流程：

```text
外部 token
    ↓
GoChat 外部认证
    ↓
获得 GoChat JWT
    ↓
通过 SDK 调用 HTTP API
    ↓
通过 SDK 建立 WebSocket
    ↓
发送消息并获得 ACK
    ↓
在线投递或离线存储
    ↓
重连后恢复消息
    ↓
查询历史消息、未读数并完成已读
```

P0 只承诺一个 GoChat API/WebSocket 实例。多实例 WebSocket 路由、跨实例在线状态和分布式消息投递不属于 P0。

### P0 工作模块

| 模块 | 目标 | 依赖 |
|---|---|---|
| P0-A 服务可靠性 | 启动、就绪、优雅关闭和依赖状态可观测 | 无 |
| P0-B 认证与安全 | 外部认证、JWT、CORS、WebSocket 鉴权形成稳定契约 | P0-A |
| P0-C 消息可靠性 | ACK、幂等、离线消息、未读和已读闭环 | P0-A、P0-B |
| P0-D SDK 与集成 | Go SDK 可用于真实外部后端集成 | P0-B、P0-C |
| P0-E 测试与交付 | 单元、集成、E2E 和 CI 门禁 | P0-A～P0-D |

## User Stories

### 外部应用集成

1. 作为外部应用开发者，我想使用 Go SDK 完成外部 token 到 GoChat token 的交换，以便不需要手写认证 HTTP 请求。
2. 作为外部应用开发者，我想使用 SDK 调用好友、群组、消息和未读接口，以便接入成本保持稳定。
3. 作为外部应用开发者，我想使用 SDK 建立 WebSocket 连接，以便不需要手写协议解析和鉴权逻辑。
4. 作为外部应用开发者，我想让 SDK 自动规范化裸 token 与 `Bearer <token>`，以便不同认证系统可以直接接入。
5. 作为外部应用开发者，我想通过 SDK 获得结构化错误，以便区分认证失败、业务失败、网络失败和服务不可用。
6. 作为外部应用开发者，我想通过标准 HTTP handler 暴露外部用户信息接口，以便 GoChat 可以复用我的现有认证系统。
7. 作为外部应用开发者，我想在 SDK 文档中看到完整的最小集成示例，以便在没有阅读 GoChat 内部代码的情况下完成接入。
8. 作为外部应用开发者，我想在 GoChat 升级后继续使用既有接口，以便不会因为非破坏性升级而修改宿主应用。

### 服务生命周期

9. 作为运维人员，我想知道服务是否存活，以便区分进程崩溃和依赖不可用。
10. 作为编排系统，我想知道服务是否已经准备好接收流量，以便避免把请求发送到尚未连接数据库的实例。
11. 作为运维人员，我想在发送停止信号后让服务停止接收新请求并释放资源，以便滚动重启不会产生大量错误。
12. 作为运维人员，我想看到明确的启动阶段日志，以便判断配置、数据库、对象存储和监听端口分别是否成功。
13. 作为运维人员，我想在关键依赖初始化失败时让应用明确失败，而不是启动后再静默报错。
14. 作为运维人员，我想知道 Redis 和 etcd 是当前部署的必需依赖还是可选依赖，以便正确配置健康检查和容量规划。

### 认证与安全

15. 作为系统管理员，我想禁止使用默认 JWT secret 启动生产配置，以便避免所有环境共享同一个签名密钥。
16. 作为外部认证系统，我想让 GoChat 在调用我的用户信息接口时保留认证语义，但不泄露 token 到日志中。
17. 作为系统管理员，我想限制外部认证接口的超时时间，以便外部系统故障不会拖垮 GoChat API。
18. 作为前端宿主应用，我想通过配置 CORS 来源，以便只允许受信任的前端页面访问 GoChat。
19. 作为 WebSocket 客户端，我想在握手阶段获得明确的未授权响应，以便能判断是 token 过期还是网络错误。
20. 作为系统管理员，我想限制认证和 WebSocket 消息的频率，以便降低暴力尝试和资源滥用风险。
21. 作为安全审计人员，我想确认访问日志和错误日志不包含 JWT、外部 token 和完整 Authorization header。

### 消息可靠性

22. 作为发送方，我想在消息持久化成功后获得 ACK，以便知道服务端已经接受了消息。
23. 作为发送方，我想用同一个 `seq` 安全重试，以便网络超时不会造成重复消息。
24. 作为接收方，我想在在线时立即收到消息，以便聊天具有实时性。
25. 作为离线用户，我想在重新连接后收到离线通知并查询离线消息，以便不会错过消息。
26. 作为接收方，我想通过历史消息接口补齐 WebSocket 断线期间的消息，以便客户端可以恢复会话。
27. 作为发送方，我想知道消息是否是重复重试产生的，以便客户端能够正确更新本地消息状态。
28. 作为接收方，我想用消息 ID 去重，以便重复投递不会在 UI 中显示两次。
29. 作为用户，我想看到未读数量，并在打开会话后清除未读状态。
30. 作为系统，我想在数据库故障时不发送成功 ACK，以便客户端可以安全重试。
31. 作为系统，我想保留现有 `seq`、`cmd`、`data` WebSocket 外壳，以便现有前端 SDK 继续工作。

### API 和协议兼容

32. 作为现有客户端，我想继续调用现有认证、好友、群组和消息接口，以便 P0 不造成破坏性升级。
33. 作为新客户端，我想使用可选的分页参数查询历史消息，以便大数据量会话不会一次性加载全部消息。
34. 作为客户端，我想获得统一的 `{code, message, data}` 响应，以便只需要实现一套错误解析逻辑。
35. 作为客户端，我想通过新增字段获得更多状态，而不需要依赖字段顺序或非正式文本。

### 测试和交付

36. 作为贡献者，我想运行 `go test ./...` 验证仓库，以便知道提交没有破坏现有模块。
37. 作为贡献者，我想运行 SDK 单元测试验证 HTTP、WebSocket 和外部用户 handler，以便快速反馈接口契约变化。
38. 作为 CI 系统，我想运行 Docker Compose E2E，以便验证真实容器网络中的外部认证和消息链路。
39. 作为发布人员，我想运行 race 检查和构建检查，以便提前发现并发错误和编译问题。
40. 作为集成使用者，我想看到失败场景的测试结果，以便知道服务在数据库、认证服务和网络故障下的行为。

## Implementation Decisions

### 1. 兼容性和部署范围

- P0 只保证单实例 API/WebSocket 部署。
- 现有 HTTP 路径、WebSocket 命令名和响应封装保持兼容。
- 允许增加字段、参数、错误码和接口；不删除或改变现有字段语义。
- 不在 P0 引入跨实例 WebSocket 路由、Redis Pub/Sub 消息转发或独立消息网关。
- 所有数据库变更采用追加式 migration，不自动删除已有表或字段。

### 2. 服务生命周期模块

应用组装层负责建立一个明确的生命周期状态：

```text
Created → Initializing → Ready → Draining → Closed
                    ↘ Failed
```

要求：

- 配置加载、配置校验、日志初始化、JWT 初始化、数据库初始化、对象存储初始化和监听器启动具有明确顺序；
- 任一 required 依赖初始化失败，应用进入 `Failed`，进程以非零状态退出；
- 收到 `SIGINT`/`SIGTERM` 后进入 `Draining`；
- `Draining` 阶段停止接受新的 HTTP/WebSocket 请求；
- 已建立的 WebSocket 连接获得关闭通知并释放资源；
- 关闭操作幂等，重复调用不会 panic；
- 使用 `/livez` 表示进程存活，使用 `/readyz` 表示是否可以接收流量；
- `/readyz` 必须报告每个 required 依赖的检查结果；Redis、etcd 如果未被当前运行路径使用，必须明确标记为 optional，不能隐式地既不初始化也不影响 readiness。

### 3. 认证和安全模块

#### JWT

- 应用启动时初始化 JWT secret；
- 生产配置使用默认 secret 时拒绝启动；
- token 解析失败统一返回未授权错误；
- 日志不得输出 token、Authorization header 或完整外部认证请求。

#### 外部认证

- GoChat 向外部用户信息接口转发认证语义；
- 外部用户接口超时不超过现有 SDK/服务约定的上限；
- 外部接口非 2xx、响应格式不合法或缺少用户 ID 时，GoChat 返回稳定的认证失败错误；
- 外部用户映射规则保持 `ext_<external_id>`；
- 认证成功后返回 GoChat JWT 和内部用户信息；
- 重复认证同一外部用户必须复用同一内部用户。

#### CORS 和 WebSocket

- CORS 来源改为配置化；
- 开发配置可以允许本地前端，生产配置不得默认允许 `*`；
- WebSocket 握手支持 Authorization header 和 query token 的现有兼容方式；
- 未授权握手返回 401，不创建连接；
- 保留现有连接级消息速率限制，并将限制行为纳入测试。

### 4. 消息可靠性模块

#### 消息发送语义

P0 使用“至少一次投递 + 服务端幂等”的语义：

- 客户端为每次发送生成 `seq`；
- `seq` 在同一个发送用户范围内唯一；
- 服务端收到消息后先校验，再持久化；
- 持久化成功后才能返回成功 ACK；
- 数据库写入失败时不得返回成功 ACK；
- 网络超时后客户端可以用相同 `seq` 重试；
- 相同发送用户和相同 `seq` 再次到达时，服务端返回原消息 ID，不重复创建消息；
- 在线投递失败不回滚已持久化消息；接收方可以通过历史消息补偿；
- 客户端必须以消息 ID 去重，不能只依赖到达次数。

#### 建议的数据契约

消息记录需要增加客户端幂等标识：

```text
sender_id      发送用户
client_seq     客户端 seq，可为空以兼容历史数据
message_id     现有雪花 ID
created_at     创建时间
```

数据库应增加 `(sender_id, client_seq)` 的唯一约束，允许历史记录的 `client_seq` 为空。

#### ACK 契约

成功 ACK 至少包含：

```json
{
  "cmd": "msg_reply",
  "seq": "client-seq-1",
  "data": {
    "status": "success",
    "message_id": "1234567890",
    "duplicate": false
  }
}
```

重复请求返回：

```json
{
  "cmd": "msg_reply",
  "seq": "client-seq-1",
  "data": {
    "status": "success",
    "message_id": "1234567890",
    "duplicate": true
  }
}
```

### 5. 离线消息、会话未读摘要和已读模块

P0 的未读模型以**会话摘要**为通知单位，而不是以单条消息为通知单位。系统不为每一条未读消息创建一条未读记录，也不在上线通知中携带全部未读消息正文。
#### 聚合粒度

P0 不新增 `conversation` 实体，也不引入 `conversation_type`/`conversation_id` 抽象。未读聚合直接使用现有用户和群组标识。

物理唯一键为：

```text
recipient_user_id + from_id + group_id
```

并使用以下约束：

- 私聊：`user_id` 是收件人，`from_id` 是对方用户 ID，`group_id = 0`；
- 群聊：`user_id` 是收件人，`from_id = 0`，`group_id` 是群组 ID；
- 群聊中最新消息的真实发送者单独保存到 `latest_sender_id`，不能再把 `from_id` 用作群聊聚合键；
- 因此群聊按 `user_id + group_id` 聚合，私聊按 `user_id + from_id` 聚合；
- `user_id + group_id` 单独不足以标识多个私聊，因为私聊的 `group_id` 都是 0，所以私聊必须保留 `from_id`。

#### 会话未读摘要字段

每个收件人、每个私聊或群聊最多一条聚合记录，至少包含：

```text
user_id                收件人用户 ID
from_id                私聊对方用户 ID；群聊固定为 0
group_id               群聊群组 ID；私聊固定为 0
unread_count           当前会话未读总数
latest_message_id      最新消息 ID
latest_sender_id       最新消息发送者
latest_message_type    最新消息类型
latest_preview         最新消息预览文本或多媒体占位信息
latest_message_at      最新消息时间
read_at                最近一次清空已读的时间
```

`latest_preview` 必须经过长度限制和敏感信息策略处理，不能因为一条超大消息导致会话列表或离线通知无限膨胀。

#### 写入语义

- 消息持久化和对应未读聚合更新必须在同一个数据库事务中完成；
- 私聊收到一条新消息时，对收件人的私聊会话聚合记录执行原子 `count + 1`，同时覆盖最新消息摘要；
- 群聊收到一条新消息时，对每个其他成员的群会话聚合记录执行原子 `count + 1`，不按发送者拆分记录；
- 用户清空一个会话后，将该会话的 `unread_count` 置零并更新 `read_at`；
- 清空之后的新消息仍然会重新增加该会话的未读数；
- 同一会话永远只保留一条当前未读聚合记录，不为每一条消息单独写 unread row；
- 总未读数通过所有会话聚合记录的 `SUM(unread_count)` 得到，或由等价的用户级汇总维护，但对外结果必须一致。

#### 离线通知契约

用户建立 WebSocket 后，服务端发送一条会话摘要通知，而不是发送全部未读消息：

```json
{
  "cmd": "offline_notify",
  "data": {
    "user_id": "42",
    "total_unread_count": 12,
    "unread_list": [
      {
        "from_id": 7,
        "group_id": 0,
        "unread_count": 5,
        "latest_message": {
          "id": "123",
          "sender_id": 7,
          "type": "text",
          "preview": "最后一条消息",
          "created_at": "2026-10-10T12:00:00Z"
        }
      }
    ]
  }
}
```

客户端收到摘要后：

1. 更新会话列表的未读角标和最后一条消息；
2. 用户打开某个会话时，再通过历史消息接口按游标加载消息；
3. 客户端按消息 ID 去重，不把离线补偿和 WebSocket 实时消息展示为两条。

#### API 兼容

- 现有按 `from_id`/`group_id` 清除未读的接口保持兼容；
- P0 使用明确的 `from_id`/`group_id` 组合语义，不新增 conversation 实体；
- `GET /api/message/unread` 返回总未读数，并可以返回按 `from_id`/`group_id` 聚合的 `unread_list`；
- `offline_notify` 保留已有命令名和 `unread_list` 字段；每个列表项按 `from_id`/`group_id` 标识一个私聊或群聊摘要；
- 历史消息接口负责返回具体消息，不由未读聚合接口返回消息列表。

#### 必须验证的规模性质

在一个会话中累计 10,000 条未读消息时：

- 未读聚合记录仍然最多只有 1 条；
- 离线通知中仍然只有 1 个会话摘要；
- 通知载荷大小与会话数量相关，而不是与未读消息数量线性相关；
- 打开会话后通过历史消息分页接口加载具体消息。

### 6. SDK 模块

后端 SDK 位于公开的 `sdk` 包，不得依赖 `internal` 包。

SDK P0 接口包括：

- 外部认证；
- 内置注册和登录；
- 用户、好友、群组、消息和未读 API；
- 文件上传；
- WebSocket 连接、发送、接收、关闭；
- `NewUserInfoHandler` 外部用户适配器；
- 结构化 `APIError`；
- `IsUnauthorized` 等错误判断帮助函数。

SDK 要求：

- HTTP client、超时、WebSocket endpoint 可配置；
- token 支持裸 token和 Bearer token；
- HTTP API 响应统一解析为 SDK 类型；
- 网络错误和业务错误不能混淆；
- WebSocket 客户端支持指数退避重连，但不自动重复发送未确认消息，除非调用者明确启用幂等重试；
- SDK 文档必须包含外部系统适配示例和最小聊天示例；
- SDK 使用语义版本，P0 期间不做破坏性公开接口修改。

### 7. API 和协议演进

允许以下非破坏性扩展：

- `GET /livez`；
- `GET /readyz`；
- 历史消息的 `limit`、`cursor` 或时间范围参数；
- ACK 中新增 `message_id`、`duplicate`；
- 离线通知新增字段；
- 错误码增加分类；
- SDK 增加新方法。

不允许：

- 删除既有接口；
- 改变既有字段含义；
- 把字符串 ID 改成数字 ID，或反之；
- 改变既有 `cmd` 的语义；
- 让旧客户端必须立即升级才能发送普通文本消息。

### 8. 推荐的实现顺序

#### P0.1：测试基线与契约

- 修复过期测试和当前模型/仓储接口不一致；
- 建立 SDK、HTTP、WebSocket 的契约测试；
- 固化外部集成 E2E；
- 记录当前 API 和 WebSocket 兼容基线。

#### P0.2：生命周期和安全

- 配置校验；
- JWT secret 校验；
- `/livez` 和 `/readyz`；
- 优雅关闭；
- CORS 白名单；
- token 日志脱敏；
- 外部认证失败和超时处理。

#### P0.3：消息幂等和离线闭环

- 增加消息幂等字段/表及 migration；
- ACK 返回消息 ID；
- 重复 seq 返回原消息 ID；
- 离线通知；
- 历史消息分页；
- 未读和已读精确处理。

#### P0.4：SDK 稳定化

- SDK API 冻结；
- WebSocket 重连；
- 错误分类；
- 集成文档；
- 外部 Go 应用示例。

#### P0.5：发布门禁

- `go test ./...`；
- `go test -race ./...`；
- `go vet ./...`；
- Docker build；
- Docker Compose E2E；
- 失败环境清理和测试报告。

## Testing Decisions

### 测试原则

测试外部行为，不测试具体实现细节。测试应该跨越尽可能高的 seam：

1. 最高 seam：外部应用 E2E，通过 Docker Compose 验证真实认证、HTTP、WebSocket、MySQL 和容器网络；
2. HTTP seam：通过 `httptest` 验证 SDK 和 API 的请求/响应契约；
3. WebSocket seam：通过真实 WebSocket server/client 验证握手、ACK、重复 seq 和关闭；
4. 应用生命周期 seam：验证启动失败、ready、draining 和 close；
5. 单元 seam：仅用于纯状态机、错误分类、token 规范化等局部逻辑。

### 必须覆盖的测试

- 配置缺失、非法配置、默认 JWT secret；
- MySQL/MinIO required 依赖不可用；
- `/livez` 和 `/readyz` 的状态变化；
- SIGTERM 后不再接受新连接；
- 外部认证成功、401、超时、坏 JSON、缺少 ID；
- 裸 token 和 Bearer token；
- token 不出现在日志；
- 正常消息发送并获得 ACK；
- DB 写入失败不返回成功 ACK；
- 相同用户和 seq 重试只生成一条消息；
- 不同用户使用相同 seq 可以分别发送；
- ACK 的 message ID 可以用于历史查询；
- 接收方在线投递；
- 接收方离线后上线补偿；
- 未读数增加、查询、清除；
- WebSocket 断开、重连和客户端去重；
- SDK APIError、Unauthorized 判断和 WebSocket 协议；
- Docker Compose 外部集成 E2E；
- race detector 下的连接和消息测试。

### 发布门禁

P0 合并前必须满足：

```text
go test ./...
go vet ./...
go test -race ./...
docker compose -f examples/external-integration-demo/compose.yaml config --quiet
examples/external-integration-demo/run-e2e.ps1
```

如果某条命令因为本地 Docker、数据库或外部环境不可用而不能运行，必须在 CI 或等价的干净环境中运行，不能只记录“本地未执行”。

## Out of Scope

以下内容不属于 P0：

- 多实例 WebSocket 和跨实例消息路由；
- Redis Pub/Sub 或 Redis Streams 作为消息总线；
- etcd 驱动的运行时服务发现；
- 严格 exactly-once 网络投递；
- 全文搜索；
- 群公告、群禁言、群主转让和复杂群权限；
- 消息撤回时间窗口；
- 推送通知、邮件和短信；
- 音视频通话；
- 多租户隔离；
- 完整管理后台；
- 高级审核和内容安全；
- 文件转码、断点续传和生命周期管理；
- 自动化水平扩缩容。

## Further Notes

### 兼容性风险

当前代码中存在历史测试与模型/仓储接口不一致的问题。P0.1 必须先清理这些不一致，否则后续可靠性测试的结果不可信。

### 依赖语义

配置中已经存在 Redis 和 etcd，但核心聊天路径当前并未完整依赖它们。P0 实施时必须明确其中一种策略：

- 将其初始化并纳入 required readiness；或
- 明确标记为 optional，并从核心 readiness 中排除；或
- 删除当前版本中没有实际用途的配置和启动依赖。

不能继续保持“配置存在、运行时不明确使用、健康检查也不明确”的状态。

### 数据迁移

所有 schema 变更应提供：

- 新环境初始化 SQL；
- 已有环境升级 SQL；
- 回滚或前向修复说明；
- 唯一索引创建失败时的诊断方式；
- 大表迁移的停机和锁风险说明。

### 交付结果

P0 完成后，外部系统应该可以只依赖：

- GoChat HTTP API；
- GoChat WebSocket 协议；
- `gochat/sdk`；
- 集成配置文档；
- Docker Compose 或等价部署方式。

外部系统不应该需要导入 GoChat 的 `internal` 包，也不应该依赖 GoChat 数据库内部表结构来完成正常聊天流程。