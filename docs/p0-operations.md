# P0 运维与生命周期说明

## 健康检查

GoChat API 暴露三个兼容/健康端点：

| 端点 | 用途 | 成功状态 |
|---|---|---|
| `/healthz` | 兼容旧健康检查 | 200 |
| `/livez` | 进程存活 | 200 |
| `/readyz` | 当前应用是否接收流量 | ready=200，not ready=503 |

应用开始退出后，`/readyz` 会切换为 503。负载均衡或编排系统应使用 `/readyz` 作为流量接入依据。

## 必需和可选依赖

P0 单实例运行路径的必需依赖是：

- MySQL：消息、用户、好友、群组和未读聚合持久化；
- MinIO 或兼容 S3 服务：文件上传，以及启动时 bucket 初始化。

Redis 和 etcd 当前仍保留配置结构，但未用于单实例核心消息路径，因此属于 optional 依赖，不参与 P0 readiness。多实例路由和服务发现不属于当前 P0 范围。

## CORS 和认证安全

开发环境可以显式配置前端来源：

```yaml
cors:
  allowed_origins:
    - http://localhost:3000
```

配置了 allowlist 后，只有匹配的 `Origin` 会获得 CORS credentials。未配置 allowlist 时仅适用于本地开发，服务会使用通配来源且不授予 credentials。

认证日志只记录请求路径、用户 ID 或错误类别，不记录完整 `Authorization` header 或 token。WebSocket 支持裸 token 和 `Bearer <token>`，非法格式会在握手阶段返回 401。

认证入口默认使用单进程 IP 固定窗口限流，默认每分钟 30 次。可以在 `auth.rate_limit_per_minute` 中调整，设置为 0 使用默认值。超过限制返回 HTTP 429 和 `Retry-After`。

外部认证请求默认超时 10 秒，可以通过 `auth.external_timeout_seconds` 调整。超时、非 200 响应、无效 JSON 或缺少用户 ID/名称都会返回统一的认证失败响应。
## 启动行为

应用启动时执行配置校验：

- API 和 WebSocket 监听地址不能为空；
- MySQL host/port/database 必须有效；
- MinIO endpoint/bucket 必须配置；
- JWT secret 不得为空；
- JWT 过期时间必须大于 0；
- 外部认证模式必须配置外部用户 API；
- auth mode 只能是 builtin 或 external。

关键依赖初始化失败时，应用返回错误并拒绝启动。

## 关闭行为

收到 `SIGINT` 或 `SIGTERM` 后：

1. 应用 readiness 切换为 false；
2. HTTP server 停止接受新请求，并等待正在执行的请求；
3. WebSocket server 关闭已有连接并停止监听；
4. MySQL 连接池和日志资源由应用 Close 释放；
5. 关闭操作幂等。

默认关闭窗口为 10 秒。

## 未读聚合迁移

旧环境应备份数据库后，执行会话级未读聚合 migration。迁移会将旧群聊记录按用户和群组聚合，并保留旧表为 `unread_count_legacy`。确认数据无误后再手工删除旧表。

不要通过 GORM 自动迁移替代该 SQL，因为旧群聊的多发送者行需要显式合并。

迁移脚本由独立 `im-migrate` 命令统一执行，旧数据库先显式登记基线，见 [数据库迁移](database-migrations.md)。

## 测试命令

默认测试不需要外部基础设施：

```bash
go test ./...
go test -race ./...
```

需要 MySQL、Redis、etcd 的测试使用 integration build tag：

```bash
go test -tags=integration ./...

# 外部应用 Docker E2E
make test-e2e
```
