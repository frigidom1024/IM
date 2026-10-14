# GoChat

[![CI](https://github.com/frigidom1024/IM/actions/workflows/p0-quality.yml/badge.svg?branch=master)](https://github.com/frigidom1024/IM/actions/workflows/p0-quality.yml)
[![Go](https://img.shields.io/badge/Go-1.25.11-00ADD8?logo=go&logoColor=white)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

GoChat 是一个轻量级即时通讯服务，为业务系统提供私信、群聊和会话管理能力。既支持独立使用，也支持接入已有用户体系，通过前端核心库和 Go SDK 集成聊天功能。

## 核心特点

- **便于集成**：支持内置登录和外部认证，提供 Vue 3 前端核心库与 Go SDK，接入已有业务系统。
- **实时聊天**：支持私信和群聊，提供历史消息查询、发送确认、自动心跳与断线重连。
- **独立会话状态**：每位用户拥有自己的会话列表、未读数、阅读状态、置顶、免打扰和草稿。
- **关系与权限管理**：支持单向好友、私信发送限制、入群审核、群角色和成员禁言。
- **消息重试与恢复**：幂等重试避免重复保存，客户端主动补拉历史和通知，恢复断线期间的状态。
- **文件与媒体**：支持图片、文件、语音和视频，提供对象存储、断点上传与媒体处理。

[接入与部署](docs/integration.md) · [领域与调用流程](docs/domain.md) · [HTTP API](docs/api/README.md) · [WebSocket](docs/websocket.md) · [Go SDK](sdk/README.md)

## 技术栈

| 部分 | 技术 |
| --- | --- |
| 后端 | Go、Gin、GORM、WebSocket、JWT |
| 存储 | MySQL 8.0、MinIO；Redis、etcd 可选 |
| 前端 | Vue 3、Vite、Pinia、Vue Router、Axios |
| 部署 | Docker Compose、Nginx |
| 数据库迁移 | golang-migrate、内嵌 SQL、迁移锁与 dirty 状态 |

## 目录结构

```text
├── cmd/
│   ├── im-server/        # HTTP API 与 WebSocket 服务入口
│   ├── im-migrate/       # 数据库初始化、升级、基线和失败修复
│   └── im-maintenance/   # 媒体维护命令
├── internal/
│   ├── application/      # 应用组装与生命周期
│   ├── api/              # HTTP 路由与处理器
│   ├── connect/          # WebSocket 连接与协议处理
│   ├── logic/            # 消息、会话、关系和群组业务
│   ├── pkg/              # 基础设施与通用工具
│   └── config/           # 配置示例
├── web/
│   ├── core/             # 可嵌入宿主系统的前端核心库
│   └── src/              # Vue 页面、API 和 Store
├── sdk/                  # 后端 Go SDK
├── examples/             # 外部应用、SDK 消费者和集成测试
├── gochat/               # Compose、MySQL 脚本和 Nginx 配置
├── docs/                 # 集成、数据库、协议和运维文档
└── scripts/              # 辅助脚本
```

## 快速开始

### Docker 部署

需要 Docker 和 Compose。以下命令在仓库根目录执行：

1. 编辑 `gochat/.env`，设置数据库、MinIO 凭证、`NGINX_PORT` 和 `SERVER_IMAGE`；本地构建可使用 `SERVER_IMAGE=gochat-server:local`。
2. 编辑 `internal/config/config-dev.yaml`。Compose 挂载此文件，容器配置需设置 `mysql.host: mysql`、`mysql.port: 3306`、`minio.endpoint: minio:9000`，并让凭证与 `.env` 一致。可参考 [容器配置示例](gochat/internal/config-dev.yaml)。
3. 构建镜像并启动基础设施：

```bash
docker compose --env-file gochat/.env -f gochat/docker-compose.yaml config --quiet
docker compose --env-file gochat/.env -f gochat/docker-compose.yaml build gochat-server
docker compose --env-file gochat/.env -f gochat/docker-compose.yaml up -d mysql redis etcd minio
```

新 MySQL 数据卷自动执行 `init.sql` 和 003–006，建立当前结构，但不会生成迁移版本记录。确认初始化脚本和数据回填完整完成后，首次登记基线 6：

```bash
docker compose --env-file gochat/.env -f gochat/docker-compose.yaml run --rm --no-deps gochat-server ./im-migrate status
docker compose --env-file gochat/.env -f gochat/docker-compose.yaml run --rm --no-deps gochat-server ./im-migrate baseline -version 6
```

已有数据卷不会补跑初始化脚本。旧库需要先备份、停写，按实际完成的版本登记基线或执行 `up`；已有迁移记录时跳过基线登记。详见 [数据库初始化与升级](docs/database.md#初始化升级与修复)。

确认数据库就绪后，启动后端与入口：

```bash
docker compose --env-file gochat/.env -f gochat/docker-compose.yaml up -d gochat-server nginx
docker compose --env-file gochat/.env -f gochat/docker-compose.yaml logs --tail=100 gochat-server
docker compose --env-file gochat/.env -f gochat/docker-compose.yaml exec -T gochat-server wget -q -O - http://127.0.0.1:8080/readyz
```

Nginx 对外监听 `NGINX_PORT`，将 `/api/` 转发到后端 8080、`/ws` 转发到 7000。该编排不包含前端静态站点；健康检查在上面的后端容器内执行。生产部署需要替换默认凭证、JWT 密钥和前端 Origin，配置 HTTPS 与 WSS。

外部认证模式和独立配置挂载步骤见 [集成指南：Docker 部署](docs/integration.md#22-docker-部署)。

### 源码运行

需要仓库 `go.mod` 指定的 Go 版本（当前为 1.25.11）、MySQL 8.0 和 MinIO。视频处理需要 `ffmpeg`；单实例可关闭 Redis、etcd。

编辑 `internal/config/config-dev.yaml`，填写宿主机可访问的数据库与 MinIO 地址、凭证和 CORS 配置。目标数据库须预先创建。对新建空库，在仓库根目录执行：

```bash
go mod download
go run ./cmd/im-migrate -config internal/config/config-dev.yaml status
go run ./cmd/im-migrate -config internal/config/config-dev.yaml up
go run -tags=etcd ./cmd/im-server -config internal/config/config-dev.yaml
```

后端启动不会自动迁移。已有业务表但无迁移记录时，`up` 会拒绝执行，需先确认旧库的实际基线。

默认 HTTP API 为 `http://localhost:8080`，WS 为 `ws://localhost:7000/ws`。另开终端检查：

```bash
curl --fail http://localhost:8080/healthz
curl --fail http://localhost:8080/readyz
```

默认 `auth.mode: builtin` 使用内置注册登录。外部认证、编译部署和完整配置示例见 [集成指南](docs/integration.md)。

### 前端运行与构建

```bash
cd web
npm ci
npm run dev
```

开发页面为 `http://localhost:3000`。`web/vite.config.js` 当前将 `/api`、`/ws` 和媒体路径代理到 `localhost:8081`，适用于该端口的统一 Nginx 入口。直接连接源码后端时，将 `/api` 目标改为 `http://localhost:8080`、`/ws` 目标改为 `ws://localhost:7000`，并按实际存储入口调整媒体代理。

```bash
npm run build       # 构建前端核心库，输出到 web/core/dist
npm run build:app   # 构建完整页面，输出到 web/dist
```

## 外部系统与 SDK 接入

外部系统提供一个可验证 bearer token 的用户信息 GET 接口，返回 `{id, name, avatar}`。客户端调用 `POST /api/auth` 换取 GoChat token，然后使用 GoChat 用户 ID 调用业务接口和建立 WS 连接。

| 接入方式 | 入口与用途 |
| --- | --- |
| 前端核心库 | [web/core/index.js](web/core/index.js)：HTTP API、WS 客户端、用户/消息/会话 Store，供 Vue 宿主应用构建界面 |
| 后端 Go SDK | [sdk/README.md](sdk/README.md)：认证、业务 API、文件上传、WS 收发和外部用户接口适配器 |
| 独立集成演示 | [examples/external-integration-demo](examples/external-integration-demo/README.md)：外部认证服务、浏览器页面和 E2E 脚本 |

前端核心库目前通过仓库源码或构建产物接入。Go SDK 导入路径为 `gochat/sdk`，外部模块可用本地 `replace` 引入。

Go SDK 的消息接收接口为持续通道：

```go
defer ws.Close()
for message := range ws.Receive() {
    // 处理聊天消息、发送确认和服务端业务错误。
    log.Printf("cmd=%s seq=%s", message.Cmd, message.Seq)
}
```

`Errors()` 提供接收与自动重连错误通道；`Receive()` 复用同一个后台读取循环和连接，断线时自动重连，`Close()` 或终止性接收失败会关闭通道。完整接入顺序和示例见 [外部系统集成指南](docs/integration.md)。

## 验证

无需真实部署数据库的 Go 检查：

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

前端检查在 `web` 目录执行：

```bash
npm test
npm run build
npm run build:app
```

需要外部依赖的测试使用隔离数据库。完整集成演示要求 Docker 和 Node.js 22.4+，运行方式及清理行为见 [E2E 演示文档](examples/external-integration-demo/README.md)。

[CI 工作流](.github/workflows/p0-quality.yml) 包含 Go 测试、竞态检测、vet、构建、两套 Compose 配置校验，以及外部应用 E2E、独立 SDK 消费者和隔离 MySQL 迁移测试。

常用快捷命令见 [Makefile](Makefile)：`make build`、`make build-migrate`、`make run`、`make test`。

## 文档

- [领域概念与接口调用流程](docs/domain.md)
- [外部系统集成与部署](docs/integration.md)
- [数据库结构、初始化与迁移](docs/database.md)
- [WebSocket 协议](docs/websocket.md)
- [HTTP API 与 OpenAPI 契约](docs/api/README.md)
- [后端 Go SDK](sdk/README.md)
- [独立 Go SDK 消费者](examples/sdk-integration/README.md)
- [外部应用集成演示](examples/external-integration-demo/README.md)
- [健康检查与 Prometheus 指标](docs/operations/runtime.md)
- [领域术语](CONTEXT.md)
- [会话状态归属设计](docs/adr/0001-viewer-owned-conversations.md)
- [产品定位](docs/PRODUCT.md)

## 贡献

欢迎提交缺陷报告、功能建议和接入问题。开发与 PR 流程见 [贡献指南](CONTRIBUTING.md)，反馈入口见 [Issue 模板](https://github.com/frigidom1024/IM/issues/new/choose)。

## License

本项目采用 [MIT 许可证](LICENSE)，允许商业使用、修改和分发，使用时保留版权及许可声明。
