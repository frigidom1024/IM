# ===== Build Stage =====
FROM golang:1.25-alpine AS builder

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=$GOPROXY

WORKDIR /build

# 先拷贝依赖文件以利用 Docker 缓存层
COPY go.mod go.sum ./
RUN go mod download

# 拷贝源码并编译
COPY . .
RUN CGO_ENABLED=0 go build -tags=etcd -o im-server ./cmd/im-server

# ===== Run Stage =====
FROM alpine:3.19

RUN apk add --no-cache tzdata ca-certificates ffmpeg

WORKDIR /app
RUN mkdir -p /app/internal/config
ENV GOCHAT_UPLOAD_DIR=/app/uploads
RUN mkdir -p /app/uploads

COPY --from=builder /build/im-server .

# 配置文件通过外部 volume 挂载
EXPOSE 8080 7000

CMD ["./im-server"]
