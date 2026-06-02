APP_NAME = gochat

.PHONY: all build run docker-build docker-up docker-down docker-logs docker-restart dev-web clean

all: build

# ========== 后端编译 ==========
build:
	cd server && go build -tags=etcd -o bin/$(APP_NAME).bin ./main.go

run:
	cd server && go run -tags=etcd ./main.go

test:
	cd server && go test -tags=etcd ./...

# ========== Docker ==========
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
