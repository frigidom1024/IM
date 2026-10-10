# 外部应用集成 E2E 演示

这个目录是一个最小化的“外部应用”，用于验证 GoChat 能否被其它应用集成。
它不依赖 Vue、Node 服务端框架或第三方运行时依赖，外部应用本身使用 GoChat Go SDK 提供用户信息接口，只提供：

- 外部用户信息接口：`GET /api/user/info`（由 `sdk.NewUserInfoHandler` 提供）
- 一个简单的浏览器页面：选择 Alice / Bob，并调用 GoChat 外部认证和 WebSocket
- 一个自动化 E2E 脚本：验证外部认证、好友接口、WebSocket 私聊和消息持久化

## 集成拓扑

```text
浏览器 / e2e.mjs
      │
      ├── external-app:18090
      │     └── GET /api/user/info
      │
      └── GoChat:18080 / 17000
            ├── POST /api/auth
            ├── GET  /api/friend/list
            ├── POST /api/friend/add
            ├── GET  /api/message/private/:id
            └── WS   /ws
```

GoChat 使用 `config.yaml` 中的配置访问容器内的外部服务。对象存储使用项目内置的轻量 S3 stub，仅用于让 MinIO 客户端完成启动时的 bucket 初始化；本 E2E 不覆盖文件上传：

```yaml
auth:
  mode: external
  external_user_api: http://external-app:8090/api/user/info
```

外部应用将以下 token 映射为用户信息：

| 外部 Token | 外部用户 ID | 昵称 |
|---|---|---|
| `demo-alice` | `external-alice` | Alice |
| `demo-bob` | `external-bob` | Bob |

## 运行自动化 E2E

要求：

- Docker Desktop 已启动
- Node.js 22.4+（脚本使用 Node 内置 WebSocket 客户端）

在仓库根目录执行：

```powershell
.\examples\external-integration-demo\run-e2e.ps1
```

脚本会：

1. 构建 GoChat 镜像和外部应用镜像；
2. 启动 MySQL、S3 stub、GoChat、外部应用；
3. 等待服务就绪；
4. 为 Alice 和 Bob 分别执行外部认证；
5. 创建 Alice → Bob 的好友关系；
6. 建立两个 WebSocket 连接并发送私聊消息；
7. 通过历史消息接口确认消息已持久化；
8. 默认使用 `docker compose down -v` 清理环境。

如果希望保留容器和数据，便于排查问题：

```powershell
.\examples\external-integration-demo\run-e2e.ps1 -KeepData
```

也可以手动启动后运行脚本：

```powershell
docker compose -f examples/external-integration-demo/compose.yaml up -d --build
node examples/external-integration-demo/e2e.mjs
```

## 打开浏览器演示

手动启动后访问：

- [外部应用页面](http://localhost:18090)
- GoChat API 健康检查：[http://localhost:18080/healthz](http://localhost:18080/healthz)

在两个浏览器标签页中分别选择 Alice 和 Bob 登录。登录成功后，页面会显示 GoChat 用户 ID；将另一方的 ID 填入接收方输入框，即可发送消息。

停止环境：

```powershell
docker compose -f examples/external-integration-demo/compose.yaml down -v
```