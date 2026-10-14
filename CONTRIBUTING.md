# 贡献指南

欢迎通过 Issue 和 Pull Request 改进 GoChat。提交前先阅读 [领域概念与调用流程](docs/domain.md)、[集成指南](docs/integration.md) 以及需要修改的 [HTTP](docs/api/README.md) 或 [WS](docs/websocket.md) 契约。

## 提交问题与建议

使用仓库的 [Issue 模板](https://github.com/frigidom1024/IM/issues/new/choose) 提交缺陷、功能建议或接入问题。缺陷报告请附上提交版本、运行环境、认证模式、最小复现步骤和实际结果；接口问题请注明 HTTP 方法与路径或 WS 命令。

日志、配置和请求样例应隐藏 token、密码及个人消息内容。集成测试使用独立测试库；迁移与停写要求见 [数据库文档](docs/database.md)。

## 本地开发

需要 `go.mod` 指定的 Go 版本，前端使用 Node.js 和 npm；集成演示的脚本需要 Node.js 22.4+。需要容器测试时安装 Docker 与 Compose。

Fork 仓库并从 `master` 创建工作分支，在仓库根目录执行 Go 检查：

```bash
go mod download
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

未设置集成测试所需的环境变量时，数据库集成用例会跳过。需要覆盖真实存储或公共接口时，按 [集成演示](examples/external-integration-demo/README.md) 准备隔离环境，并记录实际执行的检查。

前端改动在 `web` 目录执行：

```bash
npm ci
npm test
npm run build
npm run build:app
```

Compose 改动在仓库根目录校验：

```bash
docker compose --env-file gochat/.env -f gochat/docker-compose.yaml config --quiet
docker compose -f examples/external-integration-demo/compose.yaml config --quiet
```

## 提交 Pull Request

PR 默认合入 `master`。说明解决的问题、最终行为、相关 Issue 和实际验证结果，未执行的集成检查要注明。

修改 API 或 WS 行为时同步维护契约、SDK、前端与示例。修改数据库结构时增加独立迁移，不在后端启动时自动迁移。新增接口运行 `go test ./internal/api -run TestOpenAPI -count=1`，并补充对应调用文档。

保持改动集中，避免无关的格式化或生成产物。提交消息可使用 `feat:`、`fix:`、`docs:`、`test:` 或 `refactor:` 描述本次变化。

## 许可

本项目使用 [MIT 许可证](LICENSE)，贡献的代码和文档按该许可证提供。
