# GoChat Go SDK

`gochat/sdk` 是 GoChat 的后端 Go SDK，面向需要在自己的后端系统中集成 GoChat 的应用。

它提供三类能力：

1. **调用 GoChat**：外部认证、用户、好友、群组、消息、未读数和文件上传；
2. **连接 WebSocket**：发送/接收聊天消息；
3. **实现外部用户信息接口**：用 `NewUserInfoHandler` 快速适配 GoChat 的 `auth.external_user_api`。

## 安装

当前 SDK 位于本仓库的 `sdk` 包：

```go
import "gochat/sdk"
```

外部项目使用时，可以通过 Go module replace、发布到私有 Go proxy，或将该包复制到自己的模块中。

## 调用 GoChat

```go
ctx := context.Background()

client, err := sdk.NewClient("https://chat.example.com")
if err != nil {
    log.Fatal(err)
}

// externalToken 可以是裸 token，也可以是 "Bearer <token>"。
auth, err := client.ExternalAuth(ctx, externalToken)
if err != nil {
    log.Fatal(err)
}

// WithToken 返回一个绑定 GoChat JWT 的副本，不修改原 client。
userClient := client.WithToken(auth.Token)
friends, err := userClient.ListFriends(ctx)
if err != nil {
    log.Fatal(err)
}

messages, err := userClient.ListPrivateMessages(ctx, friendID)

// 分页查询历史消息
messages, err := userClient.ListPrivateMessagesPage(ctx, friendID, 50, 0)
```

SDK 会自动：

- 添加 `/api` 路径；
- 规范化 `Bearer` 前缀；
- 设置 `Authorization` 请求头；
- 解析 GoChat 的 `{code, message, data}` 响应封装；
- 将业务错误转换为 `*sdk.APIError`。

## WebSocket

如果 HTTP 和 WebSocket 共用同一个反向代理地址，不需要额外配置：

```go
ws, err := userClient.ConnectWebSocket(ctx)
if err != nil {
    log.Fatal(err)
}
defer ws.Close()

seq, err := ws.SendChat(auth.User.ID, friendID, 0, "来自后端 SDK 的消息", "text")
if err != nil {
    log.Fatal(err)
}

for {
    message, err := ws.Receive()
    if err != nil {
        log.Fatal(err)
    }

    if message.Cmd == "msg_reply" && message.Seq == seq {
        var reply sdk.MessageReply
        _ = message.DecodeData(&reply)
        break
    }
}
```

如果 API 和 WebSocket 是两个端口：

```go
client, err := sdk.NewClient(
    "http://localhost:8080",
    sdk.WithWebSocketURL("ws://localhost:7000/ws"),
)
```

## 外部系统用户信息接口

外部系统需要提供一个接口，GoChat 会把前端或外部系统传来的 bearer token 转发给它。使用 SDK 可以直接注册这个适配器：

```go
mux := http.NewServeMux()
mux.Handle("/api/user/info", sdk.NewUserInfoHandler(
    sdk.UserResolverFunc(func(ctx context.Context, token string) (sdk.ExternalUser, error) {
        user, err := myAuthService.Resolve(ctx, token)
        if err != nil {
            return sdk.ExternalUser{}, err
        }
        return sdk.ExternalUser{
            ID:     user.ID,
            Name:   user.DisplayName,
            Avatar: user.AvatarURL,
        }, nil
    }),
))
```

`UserResolver` 收到的是去掉 `Bearer ` 前缀后的裸 token，返回格式固定为：

```json
{
  "id": "external-user-42",
  "name": "Alice",
  "avatar": "https://example.com/avatar.png"
}
```

## WebSocket 自动心跳与重连

`ConnectWebSocket()` 成功后，SDK 默认每 30 秒自动发送协议 `ping`，无需额外启动定时器。心跳和业务消息串行写入；心跳写入超时为 5 秒，失败时使用已有重连逻辑恢复连接。重连后自动心跳继续运行，`Close()` 会停止心跳并取消正在进行的自动重连。

可在创建客户端时调整间隔，传入 `0` 或负值关闭自动心跳：

```go
client, err := sdk.NewClient(baseURL,
    sdk.WithToken(token),
    sdk.WithWebSocketHeartbeatInterval(15*time.Second),
)
```

调用方仍应持续调用 `Receive()` 处理业务消息和服务端控制帧。协议 `pong` 响应也由 `Receive()` 返回；手动 `Ping()` 继续可用。

SDK 的 `Receive()` 在遇到临时断线时会自动重新建立连接，最多尝试 5 次并使用退避等待。`Send()` 遇到写入失败时会尝试恢复连接，但不会自动重发原消息；调用方应使用原来的 `seq` 再次发送，利用服务端幂等保证不会产生重复消息。

调用方也可以显式执行：

```go
if err := ws.Reconnect(ctx); err != nil {
    // 连接恢复失败
}
```
## 错误处理

```go
friends, err := userClient.ListFriends(ctx)
if err != nil {
    var apiErr *sdk.APIError
    if errors.As(err, &apiErr) {
        log.Printf("GoChat error: http=%d code=%d message=%s", apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
    }
    if sdk.IsUnauthorized(err) {
        // 重新执行外部认证或刷新 GoChat JWT
    }
}
```

## 当前覆盖的 GoChat 接口

- 外部认证、内置注册和登录
- 用户信息、资料和状态
- 好友列表、添加和删除
- 群组创建、列表、加入、退出和成员管理
- 私聊/群聊历史消息
- 标记已读和未读数量
- 文件上传
- WebSocket 连接、聊天消息、ping 和原始消息收发
- 外部用户信息 HTTP handler
### 消息幂等重试

SDK 的 WebSocket `SendChat` 会生成 `seq`。调用方在没有收到 ACK 时，可以使用同一个 seq 重试原始消息；服务端会返回原 `message_id`，并在 ACK 中设置 `duplicate: true`，不会重复保存或重复增加未读数。

## SDK 0.2.0 configuration

The semantic version is `sdk.Version`; see `CHANGELOG.md`.

```go
client, err := sdk.NewClient(baseURL,
    sdk.WithTimeout(10*time.Second),
    sdk.WithTransport(http.DefaultTransport),
    sdk.WithReadRetry(2, 100*time.Millisecond),
    sdk.WithTokenRefresh(func(ctx context.Context, current string) (string, error) {
        // Check your cached expiration and refresh when needed.
        return current, nil
    }),
)
user := client.WithToken(jwt)
```

Options apply in order. Defaults: 15-second HTTP timeout, no read retries.
Bodyless GET requests may retry transport errors and HTTP 429/502/503/504,
with at most five retries and context-aware linear backoff. Writes and uploads
are never replayed. Authentication failures are returned without replay;
the refresh callback checks validity before the request or WS handshake.
It is serialized per user client. Do not reenter the same client from that
callback. `WithToken` creates independent token state for each user.

`WithLogger` accepts a `Log(context.Context, string, map[string]any)` logger.
Only fixed events, HTTP method, status, attempt, duration and transport-error
boolean are recorded; URLs, parameters, bodies, tokens and raw errors are
excluded. The supplied logger must support concurrent calls.

WebSocket sends and receives are serialized. Failed sends still require the
caller to choose whether to resend with the original seq. `Close` cancels
pending reconnect delays and handshakes. For a Receive deadline, use
`context.AfterFunc(ctx, func(){ ws.Close() })`.

## Streaming uploads

```go
meta, err := user.Upload(ctx, sdk.UploadRequest{
    File: file, Filename: "video.mp4", Type: "video", Size: size,
    Progress: func(sent, total int64) { /* update progress */ },
})
```

Multipart data is streamed with bounded buffering. Progress runs on the upload
goroutine and reports bytes read from the input, not bytes acknowledged by
storage. Size zero means unknown. The caller owns the reader; blocking readers
should honor cancellation. An early server rejection closes the upload pipe.

## Runnable adapters and independent integration consumer

Set a development `GOCHAT_EXAMPLE_TOKEN` and run `go run ./examples/sdk-nethttp`
(port 8090) or `go run ./examples/sdk-gin` (8091). Configure GoChat's
`auth.external_user_api` to point at its `/api/user/info`. Replace the fixed
example token resolver with your own authentication implementation.

`examples/sdk-integration` is a separate module consuming only the public SDK.
Run `go test -v ./...` there against a running Docker stack using the environment
variables described in its README. Without a server URL the live test explicitly
skips; this does not count as verifying the Docker server.


## P1 conversations and groups

`ListConversations(ctx, limit, offset)` includes names, avatars, unread counts,
last messages and viewer preferences. `UpdateConversation` takes the stable
`private:<user_id>` or `group:<group_id>` scope and a `ConversationPatch` with
pointer fields so false and empty draft values are distinct from omission.
Message IDs accept both legacy JSON numbers and lossless JSON strings.

Group management methods: `SearchGroups`, `InviteGroupMember`,
`TransferGroupOwner`, `UpdateGroupAnnouncement`, `UpdateGroupMember`,
`DissolveGroup`, `ListGroupMembersPage`, and `ListGroupReadReceipts`.
`GroupMemberPatch` uses optional nickname, role and muted-until fields;
set `Unmute: true` to clear the mute. Server authorization remains authoritative.
`ListGroupFiles(ctx, groupID, limit, offset)` lists group assets.

## Resumable uploads and private downloads

```go
meta, err := user.UploadFile(ctx, sdk.UploadFileRequest{
    File: file, Name: "video.mp4", Type: "video", MIME: "video/mp4", Size: size,
    Progress: func(acknowledged, total int64) { /* persisted progress */ },
})
var uploadErr *sdk.UploadSessionError
if errors.As(err, &uploadErr) {
    // Persist uploadErr.SessionID and supply it on a later UploadFile call.
    // Use the identical seekable file and Size when resuming.
}
```

A new upload creates a session; a request with `SessionID` retrieves its persisted
offset. The input must be a seekable file with the declared size. Chunks are at
most 8 MiB and use bounded memory. Failed chunks are never replayed automatically,
including ambiguous errors where the server might have stored part of a chunk.
Progress reports the initial persisted offset and each acknowledged offset,
synchronously. New sessions can declare `GroupID` and `ExpiresHours`.
A returned `UploadSessionError` wraps the original `APIError` or transport error
and preserves the session ID for resumption.

For manual control use `CreateUploadSession`, `GetUploadSession`,
`AppendUploadChunk`, `CompleteUploadSession` and `AbortUploadSession`.

```go
response, err := user.DownloadFile(ctx, fileID)
if err != nil { return err }
defer response.Body.Close()
_, err = io.Copy(destination, response.Body)
```

`DownloadThumbnail` has the same streaming response interface. These methods
send authentication, preserve response headers and convert JSON HTTP errors
into `APIError`; successful binary responses are never decoded as JSON.
The caller closes the response body. Downloads are single requests and do not
retry a partially consumed body. File IDs are the segment in the returned
`/api/file/<file_id>/download` URL.
