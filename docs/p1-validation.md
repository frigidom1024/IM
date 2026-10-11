# P1 验证记录

2026-10-11，分支 codex/p1。按用户确认的全部六块范围和公开 HTTP、独立 Go SDK、真实 WebSocket、Docker E2E 边界实现与验证。

| 阶段 | 结果与验证 |
| --- | --- |
| SDK 产品化 | 0.2.0 版本与变更记录；超时、Transport、安全 GET 重试、刷新回调、日志和 WebSocket 并发关闭；SDK race 测试通过，独立消费项目在实际服务上通过 HTTP、上传进度、下载和 WebSocket 测试 |
| 运维 | 依赖 readiness、HTTP/认证/上传/WS/消息/数据库指标、可选 Redis 池指标、统一错误；公开接口测试及 Docker metrics/readyz 检查通过 |
| 会话 | 私聊/群会话、最后消息、未读、置顶、免打扰和草稿；MySQL HTTP 测试及 Docker 用户隔离验证通过 |
| 群管理 | 搜索、邀请、容量和角色权限、公告、昵称、禁言、回执、转让和解散；实际 HTTP/WS 测试及 Docker 闭环通过 |
| 媒体 | 缩略图、时长、视频转码、鉴权下载、断点续传、群文件、过期与孤儿清理；Docker 服务重启续传、实际 ffmpeg 处理、伪造分享拒绝和可信分享授权通过 |
| API 文档 | 39 个路径、42 个 HTTP 操作的嵌入 OpenAPI；路由契约测试通过，HTTP /openapi.json 可访问；WS 契约独立记录 |

完整 `go test -race ./...`（配置两个独立 MySQL 测试库）、`go vet ./...`、`go build ./...` 通过。两个 Compose 配置校验通过。前端 `npm test` 四项测试、应用构建和 core 构建通过；Vite 保留已有动态/静态混合导入提示。

Docker 基础 E2E 和 P1 E2E 通过。过期资产首先返回410，维护后返回404；101个合法旧对象后的孤儿仍被清理，合法永久对象保留。审查发现的任意消息自授文件权限、迁移挂载、群文件遗漏、完成失败补偿竞态、孤儿扫描停滞和媒体资源限制问题均已处理。

## 复现

运行 `examples/external-integration-demo/run-e2e.ps1`，会构建隔离 Docker 服务，执行基础/P1 E2E 和独立 SDK 消费项目，最后停止并清理该测试环境。CI 同时创建独立 MySQL 数据库运行公开接口测试。仅运行 `go test ./...` 而不配置 GOCHAT_P1_TEST_DSN/GOCHAT_GROUP_TEST_DSN 时，MySQL 接口测试会跳过。

## 验证边界

对象存储使用通过真实 MinIO HTTP 客户端访问的本地 S3 替身，验证字节、Range、对象列表和删除；未验证 AWS 签名、真实 MinIO 生命周期调度和生产大文件吞吐。生命周期配置入口已实现，需在部署目标验证实际执行。前端没有完成浏览器视觉与点击验收；已有 HTTP、缓存清理、续传测试和生产构建验证。多实例共享上传不在本次单实例规格范围内。
