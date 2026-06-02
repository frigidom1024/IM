# GoChat 仓库结构优化设计

## 概述

针对 GoChat IM 系统进行仓库结构优化，实现前后端分离、统一部署编排和 Docker 容器化。当前 `web/` 目录下的 Vue 3 前端与 Go 后端代码混合在根目录，结构不够清晰。

## 设计方案

采用 **Monorepo 层次化整理** 方案：后端代码迁入 `server/`，新增 `deploy/` 编排目录，根目录保持整洁。

## 目录结构

```
d:\project\IM\
├── server/                    ← 后端代码集中迁入
│   ├── api/                   ← Gin HTTP API
│   ├── connect/               ← WebSocket 连接层
│   ├── logic/                 ← 业务逻辑 (model/registry/service)
│   ├── pkg/                   ← 公共工具包
│   ├── proto/                 ← 协议定义
│   ├── task/                  ← 定时任务
│   ├── test/                  ← 测试代码
│   ├── scripts/               ← 脚本
│   ├── config/                ← 配置文件
│   ├── main.go                ← 后端入口
│   ├── go.mod / go.sum        ← Go 模块定义
│   ├── Dockerfile             ← 后端镜像构建
│   └── .dockerignore          ← Docker 构建忽略
│
├── web/                       ← Vue 3 前端（独立 Vite 项目）
│   ├── src/
│   ├── index.html
│   ├── package.json
│   └── vite.config.js
│
├── deploy/                    ← 部署编排目录
│   ├── docker-compose.yaml    ← 全服务编排
│   ├── .env                   ← 环境变量
│   ├── mysql/
│   │   └── init.sql           ← 数据库初始化
│   ├── nginx/
│   │   └── nginx.conf         ← Nginx 反代配置
│   └── server/
│       └── config-dev.yaml    ← 后端容器配置
│
├── docs/                      ← 文档
│   ├── database.md
│   └── superpowers/specs/
│
├── Makefile                   ← 顶层编排脚本
└── .gitignore                 ← 全局忽略规则
```

## 部署架构

```
宿主机                          Docker 网络
┌─────────────────┐
│  浏览器          │
│  localhost:80    │  ← 开发走 Vite 3000 / 部署走 Nginx 80
└────────┬────────┘
         │
   ┌─────▼─────┐
   │  Nginx    │   ← docker 容器, 宿主机端口 80
   │  :80      │
   └──┬─────┬──┘
      │     │
  /api │     │ /ws
      │     │
  ┌───▼─┐ ┌─▼──────────┐
  │ Go  │ │ Go         │
  │ API │ │ WebSocket  │
  │:8080│ │ :7000      │
  └──┬──┘ └────────────┘
     │
     │ MySQL / Redis / etcd / MinIO
     └──────────────────────────────
```

## Docker 镜像

### 后端 (server/Dockerfile)

多阶段构建，Go 编译阶段 → alpine 运行阶段。配置通过 volume 挂载覆盖。

### Nginx (deploy/nginx/nginx.conf)

```nginx
server {
    listen 80;

    # API 反向代理到 Go 后端
    location /api/ {
        proxy_pass http://gochat-server:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # WebSocket 反向代理
    location /ws {
        proxy_pass http://gochat-server:7000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

前端开发时通过 `npm run dev` 运行，Vite 代理配置：
```
/api → http://localhost:80 → Nginx → gochat-server:8080
/ws  → ws://localhost:80 → Nginx → gochat-server:7000
```

## Docker Compose

`deploy/docker-compose.yaml` 编排 6 个服务：

| 服务 | 镜像 | 说明 |
|------|------|------|
| mysql | mysql:8.0 | MySQL 数据库，加健康检查 |
| redis | redis:7-alpine | Redis 缓存队列 |
| etcd | quay.io/coreos/etcd:v3.5.9 | 服务发现 |
| minio | minio/minio:latest | 对象存储 |
| gochat-server | 本地构建 | Go 后端（不暴露宿主机端口） |
| nginx | nginx:alpine | 统一入口（宿主机 80 端口） |

## go.mod 方案

保持 `module gochat`，不修改模块名。所有 import 语句零改动。`go.mod` 文件整体迁入 `server/`。

## 开发工作流

```bash
# 启动全部服务（基础设施 + 后端 + Nginx）
make docker-up

# 启动前端热更新（另一个终端）
make dev-web

# 修改后端后重新构建
make docker-build
```

## 变更清单

1. 创建 `server/`，将 `api/ connect/ logic/ pkg/ proto/ task/ test/ scripts/ config/ main.go go.mod go.sum` 移入
2. 创建 `server/Dockerfile` 和 `.dockerignore`
3. 创建 `deploy/` 及其子目录和配置文件
4. 重写 `Makefile` 为顶层编排
5. 更新 `.gitignore` 适配新路径
6. 删除原 `docker/` 目录（功能已被 `deploy/` 替代）
7. 验证编译和构建
