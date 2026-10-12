APP_NAME = im-server

.PHONY: all build build-migrate run test test-integration test-e2e docker-build docker-up docker-down docker-logs docker-restart dev-web clean

all: build

# ========== 后端编译 ==========
build:
	go build -tags=etcd -o $(APP_NAME) ./cmd/im-server

build-migrate:
	go build -o bin/im-migrate ./cmd/im-migrate

run:
	go run -tags=etcd ./cmd/im-server

test:
	go test ./...

test-integration:
	go test -tags=integration ./...

test-e2e:
	powershell -NoProfile -ExecutionPolicy Bypass -File examples/external-integration-demo/run-e2e.ps1

# ========== Docker ==========
docker-build:
	docker compose -f gochat/docker-compose.yaml build gochat-server

docker-up:
	docker compose -f gochat/docker-compose.yaml up -d

docker-down:
	docker compose -f gochat/docker-compose.yaml down

docker-logs:
	docker compose -f gochat/docker-compose.yaml logs -f

docker-restart:
	docker compose -f gochat/docker-compose.yaml restart

# ========== 前端开发 ==========
dev-web:
	cd web && npm run dev

# ========== 清理 ==========
clean:
	rm -f $(APP_NAME) $(APP_NAME).exe
