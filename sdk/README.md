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