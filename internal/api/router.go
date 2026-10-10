package api

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"gochat/internal/api/handler"
	"gochat/internal/api/middleware"
	"gochat/internal/logic/registry"
	"gochat/internal/pkg/minio"

	"github.com/gin-gonic/gin"
)

// Router API路由
type Router struct {
	engine   *gin.Engine
	serverMu sync.Mutex
	server   *http.Server
}

// NewRouter 创建路由
func NewRouter(
	userRepo registry.UserRepository,
	friendRepo registry.FriendRepository,
	groupRepo registry.GroupRepository,
	groupMemberRepo registry.GroupMemberRepository,
	messageRepo registry.MessageRepository,
	messageReadRepo registry.MessageReadRepository,
	minioClient *minio.Client,
	authMode string,
	authExternalAPI string,
	authRateLimitPerMinute int,
	externalTimeoutSeconds int,
	allowedOrigins []string,
	ready func() bool,
) *Router {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(middleware.CORS(allowedOrigins))
	engine.Use(gin.Logger(), gin.Recovery())

	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().Unix()})
	})
	engine.GET("/livez", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().Unix()})
	})
	engine.GET("/readyz", func(c *gin.Context) {
		if ready == nil || ready() {
			c.JSON(http.StatusOK, gin.H{"status": "ready"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
	})

	userHandler := handler.NewUserHandler(userRepo)
	friendHandler := handler.NewFriendHandler(friendRepo)
	groupHandler := handler.NewGroupHandler(groupRepo, groupMemberRepo)
	messageHandler := handler.NewMessageHandler(messageRepo, messageReadRepo, userRepo)
	uploadHandler := handler.NewUploadHandler(minioClient)

	apiGroup := engine.Group("/api")
	{
		auth := apiGroup.Group("/auth")
		auth.Use(middleware.RateLimit(authRateLimitPerMinute, time.Minute))
		if authMode == "external" {
			authHandler := handler.NewExternalAuthHandlerWithTimeout(userRepo, authExternalAPI, time.Duration(externalTimeoutSeconds)*time.Second)
			auth.POST("", authHandler.ExternalAuth)
		} else {
			auth.POST("/register", userHandler.Register)
			auth.POST("/login", userHandler.Login)
		}

		protected := apiGroup.Group("")
		protected.Use(middleware.AuthMiddleware())
		{
			user := protected.Group("/user")
			user.GET("/info/:id", userHandler.GetInfo)
			user.PUT("/profile", userHandler.UpdateProfile)
			user.PUT("/status", userHandler.UpdateStatus)

			friend := protected.Group("/friend")
			friend.GET("/list", friendHandler.GetList)
			friend.POST("/add", friendHandler.Add)
			friend.DELETE("/:friendId", friendHandler.Remove)

			group := protected.Group("/group")
			group.GET("/list", groupHandler.GetList)
			group.POST("/create", groupHandler.Create)
			group.POST("/:groupId/join", groupHandler.Join)
			group.DELETE("/:groupId/leave", groupHandler.Leave)
			group.GET("/:groupId/members", groupHandler.GetMembers)
			group.DELETE("/:groupId/members/:userId", groupHandler.RemoveMember)

			message := protected.Group("/message")
			message.GET("/private/:userId", messageHandler.GetPrivateMessages)
			message.GET("/group/:groupId", messageHandler.GetGroupMessages)
			message.POST("/read", messageHandler.MarkRead)
			message.GET("/unread", messageHandler.GetUnreadCount)

			protected.POST("/upload", uploadHandler.Upload)
		}
	}

	return &Router{engine: engine}
}

// Handler exposes the underlying HTTP handler for embedding and contract tests.
func (r *Router) Handler() http.Handler {
	return r.engine
}

// Run 启动服务器
func (r *Router) Run(addr string) error {
	return r.RunReady(context.Background(), addr, nil)
}

// RunReady binds the listener before reporting readiness, preventing a
// readiness race where the process is marked ready before its port is bound.
func (r *Router) RunReady(ctx context.Context, addr string, ready chan<- struct{}) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		_ = listener.Close()
		return ctx.Err()
	default:
	}
	server := &http.Server{Addr: addr, Handler: r.engine}
	r.serverMu.Lock()
	r.server = server
	r.serverMu.Unlock()
	if ready != nil {
		close(ready)
	}
	return server.Serve(listener)
}

// Shutdown 停止接收新请求并等待已有请求退出。
func (r *Router) Shutdown(ctx context.Context) error {
	r.serverMu.Lock()
	server := r.server
	r.serverMu.Unlock()
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}
