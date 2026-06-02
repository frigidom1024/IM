# GoChat 仓库结构优化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 GoChat 前端（Vue 3）和后端（Go）在仓库内分离，新增 Docker 镜像和 Compose 编排实现一键部署

**方案：** Monorepo 层次化整理 — 后端代码迁入 `server/`，新建 `deploy/` 编排目录，Nginx 统一入口，原 `docker/` 废弃

**Tech Stack:** Go 1.25, Docker, Docker Compose, Nginx, Vue 3

---

## 文件清单

### 需要移动的目录/文件（git mv）

| 源路径 | 目标路径 |
|--------|----------|
| `api/` | `server/api/` |
| `connect/` | `server/connect/` |
| `logic/` | `server/logic/` |
| `pkg/` | `server/pkg/` |
| `proto/` | `server/proto/` |
| `task/` | `server/task/` |
| `test/` | `server/test/` |
| `scripts/` | `server/scripts/` |
| `config/` | `server/config/` |
| `main.go` | `server/main.go` |
| `go.mod` | `server/go.mod` |
| `go.sum` | `server/go.sum` |

### 需要创建的文件

| 文件 | 职责 |
|------|------|
| `server/Dockerfile` | Go 后端多阶段镜像构建 |
| `server/.dockerignore` | Docker 构建忽略规则 |
| `deploy/docker-compose.yaml` | 全服务编排（6 个服务） |
| `deploy/.env` | 环境变量默认值 |
| `deploy/nginx/nginx.conf` | Nginx 反向代理配置 |
| `deploy/mysql/init.sql` | 数据库初始化（从 `docker/init.sql` 复制） |
| `deploy/server/config-dev.yaml` | 后端容器内配置（从 `config/config-dev.yaml` 复制并调整） |

### 需要修改的文件

| 文件 | 变更 |
|------|------|
| `Makefile` | 全部重写为顶层编排 |
| `.gitignore` | 适配新路径 |

### 需要删除的文件/目录

| 路径 | 原因 |
|------|------|
| `docker/` | 功能已被 `deploy/` 替代 |

---

### Task 1: 创建 server/ 目录并迁移后端代码

**Files:**
- Move: `api/` → `server/api/`
- Move: `connect/` → `server/connect/`
- Move: `logic/` → `server/logic/`
- Move: `pkg/` → `server/pkg/`
- Move: `proto/` → `server/proto/`
- Move: `task/` → `server/task/`
- Move: `test/` → `server/test/`
- Move: `scripts/` → `server/scripts/`
- Move: `config/` → `server/config/`
- Move: `main.go` → `server/main.go`
- Move: `go.mod` → `server/go.mod`
- Move: `go.sum` → `server/go.sum`

- [ ] **Step 1: 创建 server/ 目录**

```bash
mkdir server
```

- [ ] **Step 2: 使用 git mv 迁移所有后端文件和目录**

```bash
git mv api/ connect/ logic/ pkg/ proto/ task/ test/ scripts/ config/ main.go go.mod go.sum server/
```

- [ ] **Step 3: 验证迁移后代码能编译**

```bash
cd server && go build -tags=etcd ./...
```

预期输出：无错误，编译成功。

- [ ] **Step 4: 提交**

```bash
git add server/
git commit -m "refactor: 将后端代码迁入 server/ 目录

- api/connect/logic/pkg/proto/task/test/scripts/config 移入 server/
- main.go/go.mod/go.sum 移入 server/
- 保持 module gochat 不变，import 零改动"
```

---

### Task 2: 创建 server/Dockerfile 和 .dockerignore

**Files:**
- Create: `server/Dockerfile`
- Create: `server/.dockerignore`

- [ ] **Step 1: 创建 server/.dockerignore**

```dockerignore
.git/
bin/
test/
*.md
```

- [ ] **Step 2: 创建 server/Dockerfile**

```dockerfile
# ===== Build Stage =====
FROM golang:1.25-alpine AS builder

WORKDIR /build

# 先拷贝依赖文件以利用缓存
COPY go.mod go.sum ./
RUN go mod download

# 拷贝源码并编译
COPY . .
RUN CGO_ENABLED=0 go build -tags=etcd -o server ./main.go

# ===== Run Stage =====
FROM alpine:3.19

RUN apk add --no-cache tzdata ca-certificates

WORKDIR /app

COPY --from=builder /build/server .

# 配置文件通过外部 volume 挂载
EXPOSE 8080 7000

CMD ["./server"]
```

- [ ] **Step 3: 提交**

```bash
git add server/Dockerfile server/.dockerignore
git commit -m "feat: 添加 Go 后端 Docker 多阶段构建镜像"
```

---

### Task 3: 创建 deploy/ 部署目录和配置文件

**Files:**
- Create: `deploy/.env`
- Create: `deploy/docker-compose.yaml`
- Create: `deploy/nginx/nginx.conf`
- Create: `deploy/mysql/init.sql`
- Create: `deploy/server/config-dev.yaml`

- [ ] **Step 1: 创建 deploy/ 子目录**

```bash
mkdir -p deploy/nginx deploy/mysql deploy/server
```

- [ ] **Step 2: 创建 deploy/.env**

```env
# MySQL
MYSQL_ROOT_PASSWORD=root
MYSQL_USER=gochat
MYSQL_PASSWORD=gochat
MYSQL_PORT=3309

# Redis
REDIS_PORT=6379

# etcd
ETCD_PORT=2379

# MinIO
MINIO_PORT=9000
MINIO_CONSOLE_PORT=9001
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=minioadmin123

# GoChat Server
SERVER_IMAGE=gochat-server:latest
SERVER_PORT=8080
WS_PORT=7000

# Nginx
NGINX_PORT=80
```

- [ ] **Step 3: 复制并调整 MySQL 初始化脚本**

```bash
cp server/config/config-dev.yaml deploy/server/config-dev.yaml
```

```bash
cp docker/init.sql deploy/mysql/init.sql
```

- [ ] **Step 4: 创建 deploy/nginx/nginx.conf**

```nginx
server {
    listen 80;

    # API 反向代理到 Go 后端
    location /api/ {
        proxy_pass http://gochat-server:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # WebSocket 反向代理
    location /ws {
        proxy_pass http://gochat-server:7000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 3600s;
    }
}
```

- [ ] **Step 5: 创建 deploy/docker-compose.yaml**

```yaml
version: "3.8"

services:
  # ========== 基础设施 ==========
  mysql:
    image: mysql:8.0
    container_name: gochat-mysql
    restart: always
    ports:
      - "${MYSQL_PORT}:3306"
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD}
      MYSQL_DATABASE: gochat
      MYSQL_USER: ${MYSQL_USER}
      MYSQL_PASSWORD: ${MYSQL_PASSWORD}
    volumes:
      - mysql-data:/var/lib/mysql
      - ./mysql/init.sql:/docker-entrypoint-initdb.d/01-init.sql
    command: --default-authentication-plugin=mysql_native_password
    networks:
      - gochat-network
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    container_name: gochat-redis
    restart: always
    ports:
      - "${REDIS_PORT}:6379"
    volumes:
      - redis-data:/data
    networks:
      - gochat-network

  etcd:
    image: quay.io/coreos/etcd:v3.5.9
    container_name: gochat-etcd
    restart: always
    ports:
      - "${ETCD_PORT}:2379"
    environment:
      ETCD_NAME: etcd0
      ETCD_LISTEN_CLIENT_URLS: http://0.0.0.0:2379
      ETCD_ADVERTISE_CLIENT_URLS: http://etcd:2379
    networks:
      - gochat-network

  minio:
    image: minio/minio:latest
    container_name: gochat-minio
    restart: always
    ports:
      - "${MINIO_PORT}:9000"
      - "${MINIO_CONSOLE_PORT}:9001"
    environment:
      MINIO_ROOT_USER: ${MINIO_ROOT_USER}
      MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD}
    volumes:
      - minio-data:/data
    command: server /data --console-address ":9001"
    networks:
      - gochat-network

  # ========== 应用服务 ==========
  gochat-server:
    image: ${SERVER_IMAGE}
    container_name: gochat-server
    build:
      context: ../server
      dockerfile: Dockerfile
    restart: always
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_started
      etcd:
        condition: service_started
    volumes:
      - ./server/config-dev.yaml:/app/config/config-dev.yaml
    environment:
      - TZ=Asia/Shanghai
    networks:
      - gochat-network

  # ========== 统一入口 ==========
  nginx:
    image: nginx:alpine
    container_name: gochat-nginx
    restart: always
    ports:
      - "${NGINX_PORT}:80"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/conf.d/default.conf
    depends_on:
      - gochat-server
    networks:
      - gochat-network

volumes:
  mysql-data:
  redis-data:
  minio-data:

networks:
  gochat-network:
    driver: bridge
```

- [ ] **Step 6: 更新 vite.config.js 代理目标到 Nginx 端口**

Modify: `web/vite.config.js`

将代理目标从后端直连改为 Nginx 统一入口（宿主机 80 端口）：

```javascript
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 3000,
    proxy: {
      '/api': 'http://localhost:80',
      '/ws': {
        target: 'ws://localhost:80',
        ws: true
      }
    }
  }
})
```

- [ ] **Step 7: 提交**

```bash
git add deploy/ web/vite.config.js
git commit -m "feat: 添加 Docker 部署编排

- deploy/docker-compose.yaml: 6 个服务完整编排
- deploy/.env: 环境变量配置
- deploy/nginx/nginx.conf: Nginx 统一入口反代
- deploy/mysql/init.sql: 数据库初始化
- deploy/server/config-dev.yaml: 后端容器配置
- web/vite.config.js: 更新代理目标到 Nginx 端口"
```

---

### Task 4: 重写 Makefile

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: 重写 Makefile**

```makefile
APP_NAME = gochat

.PHONY: all build run docker-build docker-up docker-down dev-web clean

all: build

# ========== 后端编译 ==========
build:
	cd server && go build -tags=etcd -o bin/$(APP_NAME).bin ./main.go

run:
	cd server && go run -tags=etcd ./main.go

test:
	cd server && go test -tags=etcd ./...

# ========== Docker 构建 ==========
docker-build:
	docker compose -f deploy/docker-compose.yaml build gochat-server

docker-up:
	docker compose -f deploy/docker-compose.yaml up -d

docker-down:
	docker compose -f deploy/docker-compose.yaml down

docker-logs:
	docker compose -f deploy/docker-compose.yaml logs -f

docker-restart:
	docker compose -f deploy/docker-compose.yaml restart

# ========== 前端开发 ==========
dev-web:
	cd web && npm run dev

# ========== 清理 ==========
clean:
	rm -rf server/bin/
```

- [ ] **Step 2: 提交**

```bash
git add Makefile
git commit -m "chore: 重写 Makefile 为顶层编排

- build/run/test 自动进入 server/ 目录执行
- docker-build/docker-up/down 操作 deploy/ 编排
- dev-web 启动前端 Vite 开发服务器"
```

---

### Task 5: 更新 .gitignore

**Files:**
- Modify: `.gitignore`

- [ ] **Step 1: 重写 .gitignore**

```gitignore
# Binaries
*.exe
*.dll
*.so
*.dylib
*.bin

# Go build output
server/bin/
im-server
gochat

# Test binary
*.test
*.out

# Vendor
vendor/

# IDE
.idea/
.vscode/
*.swp
*.swo
*~

# OS
.DS_Store
Thumbs.db

# Config
config/config-prod.yaml

# Docker
deploy/mysql/data/

# Frontend
web/node_modules/
web/dist/
```

- [ ] **Step 2: 提交**

```bash
git add .gitignore
git commit -m "chore: 更新 .gitignore 适配新目录结构"
```

---

### Task 6: 删除原 docker/ 目录

**Files:**
- Delete: `docker/` (全部内容)

- [ ] **Step 1: 删除 docker/ 目录**

```bash
git rm -r docker/
```

- [ ] **Step 2: 提交**

```bash
git commit -m "chore: 移除原 docker/ 目录，已被 deploy/ 替代"
```

---

### Task 7: 验证整体构建

- [ ] **Step 1: 验证 Go 后端编译**

```bash
cd server && go build -tags=etcd ./...
```

预期输出：无错误，编译成功。

- [ ] **Step 2: 验证 Docker 镜像构建**

```bash
make docker-build
```

预期输出：`gochat-server:latest` 镜像构建成功。

- [ ] **Step 3: 提交最终验证**

```bash
git add -A
# 如果没有额外变更则跳过提交
git status
```
