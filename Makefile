APP_NAME = gochat
BUILD_DIR = bin

all:build

build:
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME).bin -tags=etcd main.go

run:
	@echo "Running $(APP_NAME)..."
	go run -tags=etcd main.go

test:
	@echo "Testing $(APP_NAME)..."
	go test -tags=etcd ./...

test-cover:
	@echo "Testing $(APP_NAME)..."
	go test -v -race -coverprofile=coverage.out   ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	@echo "Cleaning $(APP_NAME)..."
	rm -rf $(BUILD_DIR)
	rm -rf coverage.out
	rm -rf coverage.html

docker-up:
	@echo "Starting Docker Containers"
	docker-compose up -f /docker/docker-compose.yml

docker-down:
	@echo "Stopping Docker Containers"
	docker-compose down -f /docker/docker-compose.yml

docker-logs:
	@echo "Showing Docker Containers Logs"
	docker-compose -f  /docker/docker-compose.yml logs -f
deps:
	@echo "Installing Dependencies"
	go mod tidy
	go mod download