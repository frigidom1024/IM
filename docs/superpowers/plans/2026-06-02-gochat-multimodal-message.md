# 多模态消息系统实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 GoChat 添加图片/文件/语音/视频等多媒体消息能力，后端上传 MinIO、WebSocket 统一通道收发

**架构：** 后端代理上传模式 — 前端 POST 文件到后端，后端存 MinIO 后返回 FileMeta，前端通过 WebSocket 统一消息通道发送
**消息类型：** 使用字符串标识（text/image/file/voice/video），Content 字段存 FileMeta JSON

**Tech Stack:** Go 1.25, MinIO SDK, Gin, MySQL, WebSocket

---

## 文件清单

### 新建文件

| 文件 | 职责 |
|------|------|
| `server/pkg/minio/client.go` | MinIO 客户端封装（上传/删除/签名 URL） |
| `server/api/handler/upload.go` | 上传 API 处理器（文件校验 + 上传） |

### 修改文件

| 文件 | 变更 |
|------|------|
| `server/go.mod` + `go.sum` | 添加 minio-go 依赖 |
| `server/main.go` | 初始化 MinIO 客户端，注入 Router |
| `server/api/router.go` | 注册 POST /api/upload 路由 |
| `server/logic/model/message.go` | MsgType 改 string，常量改用字符串 |
| `server/connect/types.go` | ChatMsgData.Type 改 string，Validate 更新 |
| `server/connect/handler/chat.go` | 适配 string type（无需逻辑变更） |
| `server/connect/errors.go` | ErrInvalidMsgType 文案 |
| `docker/init.sql` | msg_type VARCHAR(20) + 数据迁移 |
| `deploy/mysql/init.sql` | 同上 |

---

### Task 1: 添加 MinIO 依赖并创建客户端包

**Files:**
- Modify: `server/go.mod`
- Create: `server/pkg/minio/client.go`

- [ ] **Step 1: 安装 minio-go 依赖**

```bash
cd server && go get github.com/minio/minio-go/v7
cd server && go get github.com/minio/minio-go/v7/pkg/credentials
```

- [ ] **Step 2: 创建 `server/pkg/minio/client.go`**

```go
package minio

import (
	"context"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"gochat/pkg/config"
)

// FileMeta 文件元数据
type FileMeta struct {
	URL       string  `json:"url"`
	Name      string  `json:"name"`
	Size      int64   `json:"size"`
	MIME      string  `json:"mime"`
	Thumbnail string  `json:"thumbnail,omitempty"`
	Width     int     `json:"width,omitempty"`
	Height    int     `json:"height,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
}

// Client MinIO 客户端
type Client struct {
	client *minio.Client
	bucket string
}

// NewClient 创建 MinIO 客户端
func NewClient(cfg *config.MinIOConfig) (*Client, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 MinIO 客户端失败: %w", err)
	}

	ctx := context.Background()
	if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
		// 桶已存在忽略错误
		exists, errBucket := client.BucketExists(ctx, cfg.Bucket)
		if errBucket != nil || !exists {
			return nil, fmt.Errorf("创建桶失败: %w", err)
		}
	}

	return &Client{client: client, bucket: cfg.Bucket}, nil
}

// Upload 上传文件，返回对象路径
func (c *Client) Upload(ctx context.Context, objectPath string, reader io.Reader, size int64, contentType string) (*FileMeta, error) {
	_, err := c.client.PutObject(ctx, c.bucket, objectPath, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return nil, fmt.Errorf("上传文件失败: %w", err)
	}

	return &FileMeta{
		URL:  objectPath,
		Name: path.Base(objectPath),
		Size: size,
		MIME: contentType,
	}, nil
}

// PresignedURL 生成临时访问链接
func (c *Client) PresignedURL(ctx context.Context, objectPath string, expires time.Duration) (string, error) {
	url, err := c.client.PresignedGetObject(ctx, c.bucket, objectPath, expires, nil)
	if err != nil {
		return "", fmt.Errorf("生成签名 URL 失败: %w", err)
	}
	return url.String(), nil
}

// Delete 删除文件
func (c *Client) Delete(ctx context.Context, objectPath string) error {
	return c.client.RemoveObject(ctx, c.bucket, objectPath, minio.RemoveObjectOptions{})
}
```

- [ ] **Step 3: 提交**

```bash
cd d:/project/IM && git add server/go.mod server/go.sum server/pkg/minio/
git commit -m "feat: 添加 MinIO 客户端包用于文件存储"
```

---

### Task 2: 创建上传 API 处理器

**Files:**
- Create: `server/api/handler/upload.go`

- [ ] **Step 1: 创建 `server/api/handler/upload.go`**

```go
package handler

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"gochat/pkg/logger"
	"gochat/pkg/minio"
	"gochat/pkg/snowflake"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// 文件类型路径前缀
const (
	pathImages    = "images"
	pathFiles     = "files"
	pathVoices    = "voices"
	pathVideos    = "videos"
)

// 文件大小限制（字节）
const (
	maxImageSize = 10 << 20  // 10MB
	maxFileSize  = 100 << 20 // 100MB
	maxVoiceSize = 10 << 20  // 10MB
	maxVideoSize = 200 << 20 // 200MB
)

// 允许的 MIME 类型
var allowedMIME = map[string][]string{
	"image": {"image/jpeg", "image/png", "image/gif", "image/webp"},
	"file":  {"application/pdf", "application/zip", "application/gzip", "text/plain", "application/msword", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	"voice": {"audio/mp3", "audio/wav", "audio/ogg", "audio/aac"},
	"video": {"video/mp4", "video/webm", "video/quicktime"},
}

// 类型对应的最大尺寸
var maxSizeMap = map[string]int64{
	"image": maxImageSize,
	"file":  maxFileSize,
	"voice": maxVoiceSize,
	"video": maxVideoSize,
}

// UploadHandler 文件上传处理器
type UploadHandler struct {
	minioClient *minio.Client
}

// NewUploadHandler 创建上传处理器
func NewUploadHandler(minioClient *minio.Client) *UploadHandler {
	return &UploadHandler{minioClient: minioClient}
}

// Upload 上传文件
func (h *UploadHandler) Upload(c *gin.Context) {
	msgType := c.PostForm("type")
	if msgType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "缺少 type 参数"})
		return
	}

	mimes, ok := allowedMIME[msgType]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": fmt.Sprintf("不支持的消息类型: %s", msgType)})
		return
	}

	maxSize, ok := maxSizeMap[msgType]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的消息类型"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "读取文件失败"})
		return
	}
	defer file.Close()

	// 校验文件大小
	if header.Size > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"message": fmt.Sprintf("文件过大，%s 类型最大允许 %dMB", msgType, maxSize>>20),
		})
		return
	}

	// 校验 MIME 类型
	contentType := header.Header.Get("Content-Type")
	valid := false
	for _, m := range mimes {
		if contentType == m {
			valid = true
			break
		}
	}
	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": fmt.Sprintf("不支持的文件类型: %s", contentType),
		})
		return
	}

	// 生成对象路径: images/2026/06/uuid.ext
	ext := path.Ext(header.Filename)
	objectPath := fmt.Sprintf("%s/%s/%s%s",
		msgType+"s", // images/files/voices/videos
		time.Now().Format("2006/01"),
		snowflake.MustNextID(),
		ext,
	)

	data, err := io.ReadAll(file)
	if err != nil {
		logger.Error("读取文件数据失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "读取文件失败"})
		return
	}

	meta, err := h.minioClient.Upload(c.Request.Context(), objectPath, strings.NewReader(string(data)), int64(len(data)), contentType)
	if err != nil {
		logger.Error("上传文件到 MinIO 失败", zap.String("path", objectPath), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "上传文件失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": meta})
}
```

- [ ] **Step 2: 提交**

```bash
cd d:/project/IM && git add server/api/handler/upload.go
git commit -m "feat: 添加文件上传 API 处理器"
```

---

### Task 3: 注册上传路由并集成到 main.go

**Files:**
- Modify: `server/api/router.go`
- Modify: `server/main.go`

- [ ] **Step 1: 更新 `server/api/router.go`**

在 `NewRouter` 函数签名和路由注册中添加上传功能：

```go
package api

import (
	"gochat/api/handler"
	"gochat/api/middleware"
	"gochat/logic/registry"
	"gochat/pkg/minio"  // ← 新增

	"github.com/gin-gonic/gin"
)

// Router API路由
type Router struct {
	engine *gin.Engine
}

// NewRouter 创建路由
func NewRouter(
	userRepo registry.UserRepository,
	friendRepo registry.FriendRepository,
	groupRepo registry.GroupRepository,
	groupMemberRepo registry.GroupMemberRepository,
	messageRepo registry.MessageRepository,
	messageReadRepo registry.MessageReadRepository,
	minioClient *minio.Client,  // ← 新增参数
) *Router {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(middleware.CORS())
	engine.Use(gin.Logger(), gin.Recovery())

	userHandler := handler.NewUserHandler(userRepo)
	friendHandler := handler.NewFriendHandler(friendRepo)
	groupHandler := handler.NewGroupHandler(groupRepo, groupMemberRepo)
	messageHandler := handler.NewMessageHandler(messageRepo, messageReadRepo)
	uploadHandler := handler.NewUploadHandler(minioClient)  // ← 新增

	api := engine.Group("/api")
	{
		auth := api.Group("/auth")
		{
			auth.POST("/register", userHandler.Register)
			auth.POST("/login", userHandler.Login)
		}

		protected := api.Group("")
		protected.Use(middleware.AuthMiddleware())
		{
			// ... 现有路由保持不变 ...

			// 文件上传
			protected.POST("/upload", uploadHandler.Upload)  // ← 新增
		}
	}

	engine.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	return &Router{engine: engine}
}

func (r *Router) Run(addr string) error {
	return r.engine.Run(addr)
}
```

- [ ] **Step 2: 更新 `server/main.go`**

在 main 函数中初始化 MinIO 客户端并传入 Router：

```go
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"gochat/api"
	"gochat/logic/registry"
	"gochat/logic/service"
	"gochat/pkg/config"
	"gochat/pkg/logger"
	"gochat/pkg/minio"  // ← 新增
	"gochat/pkg/mysql"
	"gochat/pkg/snowflake"

	"go.uber.org/zap"
)

var (
	configFile = flag.String("config", "config/config-dev.yaml", "config file path")
)

func main() {
	flag.Parse()

	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	if err := logger.Init(cfg.Logger.Level, cfg.Logger.Output); err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	defer logger.Sync()

	if err := mysql.Init(&cfg.MySQL); err != nil {
		log.Fatalf("初始化MySQL失败: %v", err)
	}
	defer mysql.Close()

	if err := snowflake.Init(1, 1); err != nil {
		log.Fatalf("初始化雪花ID生成器失败: %v", err)
	}

	// 初始化 MinIO 客户端
	minioClient, err := minio.NewClient(&cfg.MinIO)
	if err != nil {
		log.Fatalf("初始化 MinIO 失败: %v", err)
	}
	logger.Info("MinIO 客户端初始化成功", zap.String("bucket", cfg.MinIO.Bucket))

	userRepo := registry.NewUserRepository()
	friendRepo := registry.NewFriendRepository()
	groupRepo := registry.NewGroupRepository()
	groupMemberRepo := registry.NewGroupMemberRepository()
	messageRepo := registry.NewMessageRepository()
	messageReadRepo := registry.NewMessageReadRepository()

	messageService := service.NewMessageService(messageRepo, messageReadRepo)
	groupService := service.NewGroupService(groupRepo, groupMemberRepo)

	// 传入 minioClient
	router := api.NewRouter(
		userRepo,
		friendRepo,
		groupRepo,
		groupMemberRepo,
		messageRepo,
		messageReadRepo,
		minioClient,
	)

	wsServer := NewWsServer(messageService, groupService)

	go func() {
		addr := cfg.Server.APIAddr
		logger.Info("启动API服务器", zap.String("addr", addr))
		if err := router.Run(addr); err != nil {
			log.Fatalf("启动API服务器失败: %v", err)
		}
	}()

	go func() {
		addr := cfg.Server.ConnectAddr
		logger.Info("启动WebSocket服务器", zap.String("addr", addr))
		if err := wsServer.Run(addr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("启动WebSocket服务器失败: %v", err)
		}
	}()

	logger.Info("所有服务启动完成")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("正在关闭...")
}
```

- [ ] **Step 3: 验证编译**

```bash
cd server && go build -tags=etcd ./...
```
预期输出：无错误。

- [ ] **Step 4: 提交**

```bash
cd d:/project/IM && git add server/api/router.go server/main.go
git commit -m "feat: 注册上传路由并集成 MinIO 客户端到启动流程"
```

---

### Task 4: 升级消息模型 — msg_type 由 int8 改为 string

**Files:**
- Modify: `server/logic/model/message.go`

- [ ] **Step 1: 更新 `server/logic/model/message.go`**

```go
package model

import "time"

// 消息类型（字符串标识，可扩展）
const (
	MsgTypeText    = "text"    // 文字消息
	MsgTypeImage   = "image"   // 图片消息
	MsgTypeFile    = "file"    // 文件消息
	MsgTypeMention = "mention" // @提醒消息
	MsgTypeVoice   = "voice"   // 语音消息
	MsgTypeVideo   = "video"   // 视频消息
)

// Message 消息模型
type Message struct {
	ID        int64     `gorm:"column:id;primaryKey" json:"id"`
	FromID    int64     `gorm:"column:from_id;not null;index:idx_from" json:"from_id"`
	ToID      int64     `gorm:"column:to_id;default:0;index:idx_to" json:"to_id"`
	GroupID   int64     `gorm:"column:group_id;default:0;index:idx_group" json:"group_id"`
	MsgType   string    `gorm:"column:msg_type;type:varchar(20);not null" json:"msg_type"`
	Content   string    `gorm:"column:content;type:text;not null" json:"content"`
	IsRevoked bool      `gorm:"column:is_revoked;default:0" json:"is_revoked"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime;index:idx_created" json:"created_at"`
}

func (Message) TableName() string { return "message" }
```

- [ ] **Step 2: 提交**

```bash
cd d:/project/IM && git add server/logic/model/message.go
git commit -m "feat: 消息类型改用字符串标识(text/image/file/voice/video)"
```

---

### Task 5: 更新 WebSocket 协议类型和验证

**Files:**
- Modify: `server/connect/types.go`
- Modify: `server/connect/errors.go`
- Modify: `server/connect/handler/chat.go`

- [ ] **Step 1: 更新 `server/connect/types.go`**

```go
package connect

// AuthData 认证数据
type AuthData struct {
	Token string `json:"token"`
}

// ChatMsgData 聊天消息数据
type ChatMsgData struct {
	FromID  int64  `json:"from_id"`
	ToID    int64  `json:"to_id"`
	GroupID int64  `json:"group_id"`
	Type    string `json:"type"`    // ← string: text/image/file/voice/video/mention
	Content string `json:"content"` // 文字存原文，多媒体存 FileMeta JSON
}

// MsgReadData 消息已读数据
type MsgReadData struct {
	MessageID int64 `json:"message_id"`
}

// ErrorData 错误数据
type ErrorData struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Validate 验证聊天消息数据
func (c *ChatMsgData) Validate() error {
	if c.FromID <= 0 {
		return ErrInvalidFromID
	}
	switch c.Type {
	case "text", "image", "file", "mention", "voice", "video":
		// 所有类型 content 均不可为空
	default:
		return ErrInvalidMsgType
	}
	if c.Content == "" {
		return ErrEmptyContent
	}
	if c.ToID > 0 && c.GroupID > 0 {
		return ErrInvalidTarget
	}
	if c.ToID == 0 && c.GroupID == 0 {
		return ErrInvalidTarget
	}
	return nil
}
```

注意：`MsgTypeText` 等常量在 `model` 包中，这里需要导入 `gochat/logic/model`：

```go
import "gochat/logic/model"

// 在 Validate 中使用 model.MsgTypeText 等
```

或者直接在 connect 包中定义消息类型常量避免循环依赖。由于 `connect` 包已经依赖 `logic/model`（通过 `MessageService` 接口中的 `*model.Message`），可以安全导入。

- [ ] **Step 2: 更新 `server/connect/errors.go`**

```go
ErrInvalidMsgType = errors.New("无效的消息类型，支持: text/image/file/voice/video/mention")
```

- [ ] **Step 3: 更新 `server/connect/handler/chat.go`**

`chatData.Type` 已经是 `string`，和 `model.Message.MsgType` 匹配，类型赋值无需变更。只需确认 Validate 的调用链正确。

```go
// 在 handleChatMessage 中：
msgModel := &model.Message{
    ID:      snowflake.MustNextID(),
    FromID:  senderID,
    ToID:    chatData.ToID,
    GroupID: chatData.GroupID,
    MsgType: chatData.Type,  // string → string，自动适配
    Content: chatData.Content,
}
```

- [ ] **Step 4: 提交**

```bash
cd d:/project/IM && git add server/connect/types.go server/connect/errors.go server/connect/handler/chat.go
git commit -m "feat: 更新 WebSocket 协议类型为字符串标识"
```

---

### Task 6: 更新数据库 SQL 脚本

**Files:**
- Modify: `docker/init.sql`
- Modify: `deploy/mysql/init.sql`

- [ ] **Step 1: 修改 `docker/init.sql` 和 `deploy/mysql/init.sql`**

将消息表的 `msg_type` 从 `TINYINT` 改为 `VARCHAR(20)`：

```sql
-- 消息表
CREATE TABLE IF NOT EXISTS `message` (
  `id` BIGINT PRIMARY KEY COMMENT '消息ID(snowflake)',
  `from_id` BIGINT NOT NULL COMMENT '发送者ID',
  `to_id` BIGINT DEFAULT 0 COMMENT '接收者ID(0表示群消息)',
  `group_id` BIGINT DEFAULT 0 COMMENT '群组ID(0表示单聊)',
  `msg_type` VARCHAR(20) NOT NULL COMMENT '类型:text/image/file/voice/video/mention',
  `content` TEXT NOT NULL COMMENT '消息内容(文字原文/多媒体FileMeta JSON)',
  `is_revoked` TINYINT DEFAULT 0 COMMENT '是否撤回:0否 1是',
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_from(`from_id`),
  INDEX idx_to(`to_id`),
  INDEX idx_group(`group_id`),
  INDEX idx_created(`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='消息表';
```

- [ ] **Step 2: 提交**

```bash
cd d:/project/IM && git add docker/init.sql deploy/mysql/init.sql
git commit -m "feat: msg_type 字段改为 VARCHAR(20) 支持字符串标识"
```

---

### Task 7: 验证整体编译

- [ ] **Step 1: 全量编译**

```bash
cd server && go build -tags=etcd ./...
```

预期输出：无错误，编译成功。

- [ ] **Step 2: 运行单元测试**

```bash
cd server && go test -tags=etcd ./connect/... ./test/... -v -count=1 -timeout 60s -run "TestHandlerRegistry|TestConnectServer" 2>&1
```

预期输出：4 个测试 PASS。

- [ ] **Step 3: 提交最终状态**

```bash
cd d:/project/IM && git status
```

---

### Task 8 [前端]: 添加上传 API 模块

**Files:**
- Create: `web/src/api/upload.js`

- [ ] **Step 1: 创建 `web/src/api/upload.js`**

```javascript
import http from './http'

export const uploadApi = {
  /**
   * 上传文件
   * @param {File} file - 文件对象
   * @param {string} type - 消息类型 (image/file/voice/video)
   * @returns {Promise<{url, name, size, mime}>} FileMeta
   */
  upload(file, type) {
    const form = new FormData()
    form.append('file', file)
    form.append('type', type)
    return http.post('/upload', form, {
      headers: { 'Content-Type': 'multipart/form-data' }
    }).then(res => res.data)
  }
}
```

- [ ] **Step 2: 提交**

```bash
cd d:/project/IM && git add web/src/api/upload.js
git commit -m "feat: 添加前端文件上传 API 模块"
```
