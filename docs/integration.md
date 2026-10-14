# GoChat 外部系统集成指南

本文按接入顺序介绍外部认证接口、后端部署，以及前端核心库和后端 Go SDK。外部系统负责验证自己的用户凭证；GoChat 负责本地用户映射、消息、会话、好友和群组。

业务对象、发送权限、已读语义及接口调用顺序见 [领域概念与接口调用指南](domain.md)。

## 1. 外部系统需要提供的认证接口

### 1.1 用户信息接口

外部系统需要提供一个 GoChat 后端可以访问的 HTTP GET 接口。路径可以自定义，下面以 `/api/user/info` 为例：

```http
GET /api/user/info HTTP/1.1
Authorization: Bearer <外部系统 token>
```

接口应验证 token 的有效性、有效期和用户状态，成功时返回 HTTP 200 和直接的 JSON 对象：

```json
{
  "id": "42",
  "name": "张三",
  "avatar": "https://app.example.com/avatar/42.png"
}
```

| 字段 | 类型 | 要求 |
| --- | --- | --- |
| `id` | string | 必填、非空；外部系统内稳定且唯一的用户标识 |
| `name` | string | 必填、非空；显示名称 |
| `avatar` | string | 可选；头像 URL，可返回空字符串 |

返回对象不使用 GoChat 的 `{code, message, data}` 包装。无效或过期的 token 应返回 401；GoChat 将外部接口的非 200、超时、无效 JSON 和缺少必填字段统一视为认证失败。

外部接口的地址配置在 `auth.external_user_api`。GoChat 在处理认证请求时调用它，原样转发收到的 `Authorization` 头；默认请求超时为 10 秒，可通过 `auth.external_timeout_seconds` 调整。此接口由 GoChat 后端调用，必须从后端所在主机或容器网络可达。

外部系统自行提供登录、登出和凭证刷新功能。GoChat 不要求外部 token 必须是 JWT，也不直接使用外部系统的签名密钥验证它。

### 1.2 换取 GoChat token

客户端先在外部系统登录，再向 GoChat 发起认证请求：

```bash
curl -X POST http://localhost:8080/api/auth \
  -H 'Authorization: Bearer <外部系统 token>'
```

认证过程为：

```text
客户端携带外部 token → POST GoChat /api/auth
GoChat 转发 Authorization → GET 外部用户信息接口
外部系统验证 token → 返回 id、name、avatar
GoChat 查找或创建本地用户 → 返回 GoChat token 和用户信息
```

GoChat 成功响应示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "token": "<GoChat JWT>",
    "user": {
      "id": 1,
      "name": "张三",
      "avatar": "https://app.example.com/avatar/42.png",
      "username": "ext_42"
    }
  }
}
```

后续 GoChat HTTP 请求和 WebSocket 连接使用 `data.token`。外部 `id: "42"` 映射为本地 `username: "ext_42"`；消息、好友和群成员关联使用返回的 GoChat `user.id`，不能直接使用外部用户 ID。用户首次认证时自动创建本地记录，后续认证同步非空的名称和头像。

配置 `auth.mode: external` 后提供 `POST /api/auth`；内置注册、登录路由不启用。`auth.mode: builtin` 使用 `/api/auth/register` 和 `/api/auth/login`，不提供外部认证路由。

## 2. 部署 GoChat 后端

后端需要 MySQL 8.0 和可访问的 MinIO。Redis、etcd 在单实例部署中可关闭，通过 `enabled: true` 显式启用。源码部署处理视频时还需要系统中的 `ffmpeg`；仓库 Docker 镜像已包含它。

以下示例使用 HTTP 8080、WebSocket 7000，外部用户服务监听 8090。域名、端口和凭证需按实际环境修改。Shell 命令使用 Bash；Windows 可执行相应命令，编译产物增加 `.exe` 后缀。

### 2.1 源码部署

**准备配置。** 安装仓库 `go.mod` 要求的 Go 版本（当前为 1.25.11），在仓库根目录创建 `deploy/config.yaml`：

```yaml
environment: production
cors:
  allowed_origins:
    - http://localhost:3000
server:
  api_addr: ":8080"
  connect_addr: ":7000"
mysql:
  host: 127.0.0.1
  port: 3306
  user: gochat
  password: "replace-with-db-password"
  database: gochat
minio:
  endpoint: 127.0.0.1:9000
  access_key_id: "replace-with-minio-access-key"
  secret_access_key: "replace-with-minio-secret-key"
  bucket: gochat
  use_ssl: false
redis:
  enabled: false
etcd:
  enabled: false
auth:
  mode: external
  external_user_api: "http://127.0.0.1:8090/api/user/info"
  external_timeout_seconds: 10
  rate_limit_per_minute: 30
jwt:
  secret: "replace-with-a-long-random-secret"
  expire_time: 24
logger:
  level: info
  output: stdout
```

替换所有凭证占位值，将 `cors.allowed_origins` 改为实际前端 Origin；MinIO 开启 TLS 时设置 `use_ssl: true`。GoChat 会检查或创建配置的 bucket，存储凭证需要具备相应权限。当前配置由 YAML 加载，不能通过 `AUTH_MODE`、`AUTH_EXTERNAL_API` 等环境变量覆盖。

**编译。** 在仓库根目录执行：

```bash
mkdir -p bin
go mod download
go build -tags=etcd -o bin/im-server ./cmd/im-server
go build -o bin/im-migrate ./cmd/im-migrate
```

**初始化或升级数据库。** 目标数据库必须预先创建，并给配置中的用户授予建表、修改表和业务读写权限。例如使用 MySQL 管理账号执行：

```sql
CREATE DATABASE gochat CHARACTER SET utf8mb4;
CREATE USER 'gochat'@'%' IDENTIFIED BY 'replace-with-db-password';
GRANT ALL PRIVILEGES ON gochat.* TO 'gochat'@'%';
```

新建空库执行：

```bash
./bin/im-migrate -config deploy/config.yaml status
./bin/im-migrate -config deploy/config.yaml up
./bin/im-migrate -config deploy/config.yaml status
```

确认版本为 6、`dirty: false`。`im-migrate` 内嵌 SQL，无需手动执行 `init.sql`；后端启动不会自动迁移。

已有库升级前备份并停写。有迁移记录时直接执行 `up`；没有记录时先根据实际结构与历史数据处理情况登记基线。例如确认已完成 001–005 的旧库：

```bash
./bin/im-migrate -config deploy/config.yaml baseline -version 5
./bin/im-migrate -config deploy/config.yaml up
```

基线登记不执行缺失 SQL，不能凭猜测填写版本。初始化、失败修复和迁移细节见 [数据库文档](database.md#初始化升级与修复)。

**启动并检查。** 外部认证服务和 MinIO 就绪后执行：

```bash
./bin/im-server -config deploy/config.yaml
```

在另一终端检查：

```bash
curl --fail http://localhost:8080/healthz
curl --fail http://localhost:8080/readyz
```

`healthz` 检查进程存活；`readyz` 检查监听器和已启用的依赖。外部认证接口还需要通过一次 `POST /api/auth` 验证。正式运行时通过进程管理器管理服务，并将 `/api/` 和 `/ws` 反向代理到对应监听端口。

### 2.2 Docker 部署

仓库提供 [Dockerfile](../Dockerfile) 和 [Compose 编排](../gochat/docker-compose.yaml)，包含后端、MySQL、MinIO、Redis、etcd 和 Nginx。镜像同时包含 `im-server`、`im-migrate`、`im-maintenance`。

**准备容器配置。** 将上面的配置另存为仓库根目录的 `deploy/config-docker.yaml`，修改容器内地址：

```yaml
mysql:
  host: mysql
  port: 3306
  user: gochat
  password: "与 gochat/.env 的 MYSQL_PASSWORD 一致"
  database: gochat
minio:
  endpoint: minio:9000
  access_key_id: "与 gochat/.env 的 MINIO_ROOT_USER 一致"
  secret_access_key: "与 gochat/.env 的 MINIO_ROOT_PASSWORD 一致"
  bucket: gochat
  use_ssl: false
auth:
  mode: external
  external_user_api: "http://external-app:8090/api/user/info"
  external_timeout_seconds: 10
  rate_limit_per_minute: 30
```

这是需要替换的配置片段，其余 `server`、`jwt`、`cors` 等字段仍保留完整配置中的值。`external-app` 必须是同一 Docker 网络中可解析、可访问的服务名；也可使用后端容器可达的实际域名。外部服务在宿主机上时，Docker Desktop 可使用 `host.docker.internal`；Linux 环境需配置对应 host-gateway 或使用实际可达地址。容器中的 `localhost` 指向容器本身。

编辑 `gochat/.env`，设置 MySQL、MinIO 凭证和端口；本地构建的镜像名可设为 `SERVER_IMAGE=gochat-server:local`。配置中的数据库用户与密码须分别对应 `MYSQL_USER`、`MYSQL_PASSWORD`。MySQL 初始化凭证只对新数据卷生效。

原 Compose 挂载的是 `internal/config/config-dev.yaml`。为使用独立部署配置，在 `gochat/compose.external.yaml` 添加覆盖：

```yaml
services:
  gochat-server:
    volumes:
      - ../deploy/config-docker.yaml:/app/internal/config/config-dev.yaml:ro
```

**构建与初始化。** 以下命令切换到 `gochat` 目录，使 Compose 自动读取该目录的 `.env`：

```bash
cd gochat
docker compose -f docker-compose.yaml -f compose.external.yaml config --quiet
docker compose -f docker-compose.yaml -f compose.external.yaml build gochat-server
docker compose -f docker-compose.yaml -f compose.external.yaml up -d mysql redis etcd minio
```

新 MySQL 数据卷会依次执行 `init.sql` 和 003–006，生成版本 6 的结构，但不生成迁移版本记录。确认首次初始化日志成功、相关数据回填完成后，登记基线并检查：

```bash
docker compose -f docker-compose.yaml -f compose.external.yaml run --rm --no-deps gochat-server ./im-migrate status
docker compose -f docker-compose.yaml -f compose.external.yaml run --rm --no-deps gochat-server ./im-migrate baseline -version 6
docker compose -f docker-compose.yaml -f compose.external.yaml run --rm --no-deps gochat-server ./im-migrate up
```

上面的 `baseline -version 6` 仅适用于已完整执行当前初始化脚本且无迁移记录的新库。已有数据卷不会补跑初始化脚本；旧库先停写、备份，通过 `status` 确认实际版本，无记录时登记实际基线，再执行 `up`。已有版本记录时跳过 `baseline`；dirty 失败状态按数据库文档人工修复。

**启动后端与入口。** 完成迁移并确认外部服务可达后执行：

```bash
docker compose -f docker-compose.yaml -f compose.external.yaml up -d gochat-server nginx
docker compose -f docker-compose.yaml -f compose.external.yaml logs --tail=100 gochat-server
docker compose -f docker-compose.yaml -f compose.external.yaml exec -T gochat-server wget -q -O - http://127.0.0.1:8080/readyz
```

对外入口为 `.env` 中的 `NGINX_PORT`，`/api/` 代理到 8080、`/ws` 代理到 7000。现有 Nginx 配置没有代理 `/readyz`，因此上面的检查直接在后端容器执行。8080、7000 是容器内端口，现有服务没有直接发布它们；`.env` 中的 `SERVER_PORT`、`WS_PORT` 不会自动增加端口映射。

部署前端时，宿主前端的域名也需要将 `/api/` 和 `/ws` 代理到 GoChat 入口。HTTPS 页面使用 WSS。当前 Compose 只提供后端入口，不包含前端静态站点部署。

## 3. 前端 SDK：GoChat Core

### 3.1 获取和构建

前端 SDK 的实际入口是 [web/core/index.js](../web/core/index.js)，基于 Vue 3、Pinia、Axios。它导出 HTTP API、WebSocket 客户端、用户/消息/会话 Store 和 `useChat`，供宿主系统自行构建聊天界面。

当前仓库没有配置可直接安装的 `@gochat/vue-chat` 或 `@gochat/core` npm 发布包。可使用仓库源码，或先构建核心库：

```bash
# 从仓库根目录执行
cd web
npm ci
npm run build
```

产物为 `web/core/dist/gochat-core.js`（ESM）和 `gochat-core.umd.cjs`（UMD）。Vue、Pinia、Axios 为外部依赖，宿主项目需要安装，并使用同一套 Vue/Pinia 实例：

```bash
npm install vue pinia axios
```

下面示例假设将 ESM 产物复制到宿主项目的 `src/vendor/gochat-core.js`，由宿主构建工具打包。仓库 `ChatWidget.vue` 当前只提供简化侧栏，也未从核心库导出；完整页面可参考 [Chat.vue](../web/src/views/Chat.vue)。

### 3.2 配置入口与认证

宿主 Vue 应用先安装 Pinia：

```js
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'

createApp(App).use(createPinia()).mount('#app')
```

核心库 HTTP 默认访问当前站点 `/api`，WebSocket 默认访问当前站点 `/ws`。推荐在宿主站点配置同源代理；本地直连源码部署时，宿主 Vite 可配置：

```js
server: {
  proxy: {
    '/api': { target: 'http://localhost:8080', changeOrigin: true },
    '/ws': { target: 'ws://localhost:7000', ws: true, changeOrigin: true }
  }
}
```

经过 Docker Nginx 入口时，两个代理目标都改为 `NGINX_PORT` 对应的地址。跨域直连 HTTP 可设置导出的 `http.defaults.baseURL`，但当前 WS 客户端仍使用当前页面域名，需保留 `/ws` 代理。

在聊天页面初始化 Store 后，用外部 token 换取 GoChat token：

```js
import {
  useUserStore, useChatStore, useConversationStore
} from './vendor/gochat-core.js'

const user = useUserStore()
const chat = useChatStore()
const conversations = useConversationStore()

// externalToken 由宿主系统的登录流程取得。
await user.externalAuth(`Bearer ${externalToken}`)
await conversations.loadConversations()
```

`externalAuth` 会保存 GoChat token 和本地用户信息、连接 WebSocket，并主动加载好友、群组与通知。切换用户前先调用 `user.logout()` 关闭旧连接和清理原身份。当前核心库使用 `localStorage` 的 `token`、`user` 键；宿主系统的外部凭证应使用不同键名。HTTP 401 会清除这两个键并跳转 `/login`，宿主项目需要提供对应登录入口。

### 3.3 会话、消息与通知

| 能力 | 接口或方法 |
| --- | --- |
| 会话列表、摘要、未读和偏好 | `useConversationStore()`、`conversationApi` |
| 打开会话、消息缓存、发送状态 | `useChatStore()` |
| 当前用户、好友、群组、通知 | `useUserStore()` |
| 原始 WS 订阅与发送 | `wsClient.on()`、`wsClient.sendChat()` |
| 上传与媒体缓存 | `uploadApi`、`createMediaCache` |

页面从 `conversations.conversations` 展示会话，ID 为 `private:<GoChat 对方用户ID>` 或 `group:<群ID>`。打开会话与发送消息：

```js
const item = conversations.conversations[0]
if (item) {
  const [type, id] = item.id.split(':')
  await chat.openChat(Number(id), item.name, type)
  // 历史消息加载错误由会话 Store 暴露，展示后再允许用户操作。
  if (conversations.conversationError) {
    throw new Error(conversations.conversationError)
  }
}

// 在用户点击发送时调用，发送权限以会话 send_policy 为准。
chat.sendMessage('你好', 'text')
```

`openChat` 会打开会话、加载摘要和历史，并在当前会话可见且历史加载成功后通过 HTTP 标记整个会话已读。会话 Store 管理未读与偏好，消息 Store 管理消息缓存与发送状态。

使用底层 API 时，WS 聊天发送只传一个 `conversation_id`，不传 `from_id`、`to_id`、`group_id`：

```js
import { wsClient } from './vendor/gochat-core.js'

const stop = wsClient.on('msg', message => {
  console.log(message.data.conversation_id, message.data.sender_id)
})
const seq = wsClient.sendChat('private:2', '你好', 'text')
// 页面不再需要此订阅时调用 stop()；退出登录调用 user.logout()。
```

私信双方的 `private:<对方ID>` 不同，是各自用户视角的标识；发送者由认证连接确定。消息 ID、WS `sender_id` 按字符串处理，避免大整数精度丢失。完整字段见 [WebSocket 协议](websocket.md)。

连接建立后不推送离线通知。现有 Store 会在连接成功、重连和页面重新可见时主动恢复会话、当前历史及关系通知；自行封装客户端时也应安排这些 HTTP 查询：

| 数据 | HTTP 接口 |
| --- | --- |
| 会话列表 | `GET /api/conversation` |
| 未读汇总 | `GET /api/conversation/unread` |
| 消息历史 | `GET /api/conversation/{scope}/messages` |
| 整个会话已读 | `POST /api/conversation/{scope}/read` |
| 好友通知 | `GET /api/friend/incoming` |
| 自己的入群申请 | `GET /api/group/applications/mine` |
| 管理员处理的入群申请 | `GET /api/group/{id}/applications` |

在线 `friend_changed`、`group_changed` 事件可触发刷新；持久状态仍从 HTTP 查询。完整接口见 [HTTP 契约](api/README.md)。

## 4. 后端 SDK：Go SDK

### 4.1 引入 SDK

后端 SDK 位于 [sdk](../sdk/README.md)，模块导入路径为 `gochat/sdk`。外部项目可通过本地 `replace` 引入仓库，例如：

```go
module example.com/my-app

go 1.25.11

require gochat v0.0.0

replace gochat => ../IM
```

`../IM` 是相对外部项目 `go.mod` 的仓库路径，按实际目录修改，然后执行 `go mod tidy`。可运行的独立消费者见 [examples/sdk-integration](../examples/sdk-integration/README.md)。

### 4.2 实现外部认证接口

外部系统使用 Go 时，可以用 SDK 适配已有认证逻辑：

```go
mux.Handle("/api/user/info", sdk.NewUserInfoHandler(
    sdk.UserResolverFunc(func(ctx context.Context, token string) (sdk.ExternalUser, error) {
        user, err := myAuthService.Resolve(ctx, token)
        if err != nil {
            return sdk.ExternalUser{}, err
        }
        return sdk.ExternalUser{
            ID: user.ID, Name: user.DisplayName, Avatar: user.AvatarURL,
        }, nil
    }),
))
```

`mux` 是外部应用的 `http.ServeMux`；`myAuthService.Resolve` 代表业务系统已有的 token 验证方法。回调收到去掉 `Bearer ` 前缀后的 token，必须完成真实身份验证；SDK 负责输出第 1 节要求的 JSON。Gin 适配示例见 [examples/sdk-gin](../examples/sdk-gin/main.go)，标准库示例见 [examples/sdk-nethttp](../examples/sdk-nethttp/main.go)。

### 4.3 认证、会话和消息

```go
client, err := sdk.NewClient("http://localhost:8080",
    sdk.WithWebSocketURL("ws://localhost:7000/ws"),
)
if err != nil { return err }

auth, err := client.ExternalAuth(ctx, externalToken)
if err != nil { return err }
userClient := client.WithToken(auth.Token)

conversations, err := userClient.ListConversations(ctx, 50, 0)
if err != nil { return err }
for _, item := range conversations {
    log.Printf("会话=%s 未读=%d", item.ID, item.UnreadCount)
}

scope := "private:2" // 2 是对方的 GoChat 用户 ID。
if err := userClient.OpenConversation(ctx, scope); err != nil { return err }
messages, err := userClient.ListPrivateMessagesPage(ctx, 2, 50, 0)
if err != nil { return err }
// 交给业务调用方展示或处理后，再决定是否标记整个会话已读。
log.Printf("历史消息数量=%d", len(messages))
```

需要标记整个会话已读时调用 `userClient.MarkConversationRead(ctx, scope)`。SDK 不自动决定业务调用方是否已阅读消息。多用户后端分别使用 `client.WithToken(...)` 创建各用户客户端，避免混用身份。

经过同一个反向代理入口时，只给 `NewClient` 传 HTTP 基地址即可，SDK 自动推导 `/ws`；HTTP 和 WS 分端口时才使用 `WithWebSocketURL`。地址传站点根路径，SDK 自动添加 `/api`。

### 4.4 持续接收 WebSocket 消息

```go
ws, err := userClient.ConnectWebSocket(ctx)
if err != nil { return err }
defer ws.Close()

seq, err := ws.SendChat(scope, "来自后端 SDK 的消息", "text")
if err != nil { return err }
log.Printf("发送序列=%s", seq)

incoming, receiveErrors := ws.Receive(), ws.Errors()
for incoming != nil || receiveErrors != nil {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case message, ok := <-incoming:
        if !ok { incoming = nil; continue }
        switch message.Cmd {
        case "msg":
            var chat sdk.ChatMessage
            if err := message.DecodeData(&chat); err != nil { return err }
            log.Printf("会话=%s 内容=%s", chat.ConversationID, chat.Content)
        case "msg_reply":
            var reply sdk.MessageReply
            if err := message.DecodeData(&reply); err != nil { return err }
            log.Printf("确认序列=%s 消息=%d", message.Seq, reply.MessageID)
        case "error":
            // 服务端业务错误也通过消息通道返回，应用按 seq 关联发送请求。
            log.Printf("发送失败：seq=%s data=%v", message.Seq, message.Data)
        }
    case err, ok := <-receiveErrors:
        if !ok { receiveErrors = nil; continue }
        log.Printf("接收异常：%v", err)
    }
}
```

`Receive()` 返回持续消息通道，内部只有一个读取循环，正常收发复用已建立的连接；重复调用返回同一个通道，多个消费者分摊消息。`Errors()` 报告读取、解码和自动重连异常；服务器返回的业务 `error` 命令属于消息通道。调用方应持续消费，消息缓冲满后读取会等待消费者。

SDK 默认每 30 秒发送协议心跳，断线自动重连；`Close()` 或不可恢复的接收失败会关闭两个通道。`Send` 写入失败会尝试恢复连接，但不自动重发原消息。重试时保存原非空 `seq`，通过 `ws.Send(sdk.WSMessage{Cmd: "msg", Seq: seq, Data: ...})` 发送同一内容，利用服务端幂等约束；再次调用 `SendChat` 会产生新的序列号。

后端 SDK 连接成功或恢复后也需要主动调用 `ListConversations`、`GetUnreadSummary`、历史和通知方法，不能依赖离线推送补齐状态。token 刷新、文件上传、会话偏好及其他功能见 [Go SDK 文档](../sdk/README.md)。

## 5. 可运行的完整示例

仓库提供 [外部应用演示](../examples/external-integration-demo/README.md)，包含外部用户信息接口、浏览器页面、GoChat 和独立数据库。可在仓库根目录启动：

```bash
docker compose -f examples/external-integration-demo/compose.yaml up -d --build
```

访问 `http://localhost:18090` 选择 Alice/Bob，API 地址为 `http://localhost:18080`，WS 为 `ws://localhost:17000/ws`。演示使用固定 token 和 S3 stub 验证集成链路；正式部署接入自己的用户验证逻辑和 MinIO。
