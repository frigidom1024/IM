# ===== Build Stage =====
FROM golang:1.25-alpine AS builder

WORKDIR /build

# 先拷贝依赖文件以利用 Docker 缓存层
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

# 配置文件通过外部 volume 挂载，不打包进镜像
EXPOSE 8080 7000

CMD ["./server"]
