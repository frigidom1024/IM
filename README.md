# GoChat

轻量级即时通讯（IM）系统，提供实时消息通信。核心体验简洁直接：快速可靠的消息传递、直观的对话管理，无多余装饰。

## 功能特性

- 💬 实时聊天：基于 WebSocket 的消息推送与离线消息处理
- 👥 好友 / 群组管理
- 🛡️ 双认证模式：内置注册登录，或接入外部系统认证（OAuth 风格）
- 🔌 微服务集成：可作为服务嵌入其他系统，前端通过 ChatWidget 组件快速接入
- 📎 文件上传：基于 MinIO 的对象存储
- 🐳 一键部署：Docker Compose 编排全部依赖

## 技术栈

| 端 | 技术 |
|----|------|
| 后端 | Go · Gin · GORM · WebSocket · gRPC/protobuf · JWT |
| 存储 | MySQL · Redis · etcd · MinIO |
| 前端 | Vue 3 · Vite · Pinia · Vue Router |
| 部署 | Docker Compose · Nginx |

## 目录结构

```
├── cmd/
│   └── im-server/       # 后端运行入口
├── internal/             # Go 后端实现（仅供本仓库内部使用）
│   ├── application/      # 应用组装与运行生命周期
│   ├── api/              # HTTP API 与处理器
│   ├── connect/          # WebSocket 连接层
│   ├── logic/            # 领域模型、仓储与业务服务
│   ├── pkg/              # 基础设施与通用工具
│   └── config/           # 配置文件
├── web/                  # Vue 3 前端
├── proto/                # protobuf 定义
├── sdk/                  # 后端 Go SDK（认证、API、WebSocket、外部用户适配器）
├── examples/             # 外部系统集成示例与 E2E 测试
├── gochat/               # Docker Compose 编排及配置
├── docs/                 # 文档（数据库、WebSocket 协议、集成指南等）
└── scripts/              # 脚本
```

## 快速开始

### Docker 部署（推荐）

```bash
docker compose -f gochat/docker-compose.yaml up -d
```

会启动 MySQL、Redis、etcd、MinIO、GoChat 服务及 Nginx 入口。也可通过 `Makefile` 快捷操作：

```bash
make docker-up        # 启动
make docker-logs      # 查看日志
make docker-down      # 停止
make docker-restart   # 重启
```

### 本地开发

后端默认从 `internal/config/config-dev.yaml` 加载配置：

```bash
go run -tags=etcd ./cmd/im-server
```

也可以通过 `-config` 指定配置文件：

```bash
go run -tags=etcd ./cmd/im-server -config internal/config/config-dev.yaml
```

前端：

```bash
cd web && npm install && npm run dev
```

常用命令（见 [Makefile](Makefile)）：

```bash
make build     # 编译后端
make test      # 运行无外部依赖的测试
make test-integration # 运行需要 MySQL/Redis/etcd 的集成测试
make run       # 直接运行后端
```

## 外部系统集成

GoChat 支持作为微服务集成到其他系统，外部系统只需提供一个「获取用户信息」的 API 即可打通认证。详见 [docs/integration.md](docs/integration.md)。

## CI 质量门禁

GitHub Actions 会自动执行：

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `go build ./...`
- 两套 Docker Compose 配置校验

工作流文件：`.github/workflows/p0-quality.yml`

## 文档

- [产品定位](docs/PRODUCT.md)
- [数据库设计](docs/database.md)
- [WebSocket 协议](docs/websocket.md)
- [外部系统集成指南](docs/integration.md)
- [外部集成 E2E 演示](examples/external-integration-demo/README.md)
- [后端 Go SDK](sdk/README.md)
- [P0 运维与生命周期](docs/p0-operations.md)

## License

待补充
