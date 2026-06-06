package api

import (
	"gochat/api/handler"
	"gochat/api/middleware"
	"gochat/logic/registry"
	"gochat/pkg/minio"

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
	minioClient *minio.Client,
) *Router {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(middleware.CORS())
	engine.Use(gin.Logger(), gin.Recovery())

	// 创建处理器
	userHandler := handler.NewUserHandler(userRepo)
	friendHandler := handler.NewFriendHandler(friendRepo)
	groupHandler := handler.NewGroupHandler(groupRepo, groupMemberRepo)
	messageHandler := handler.NewMessageHandler(messageRepo, messageReadRepo, userRepo)
	uploadHandler := handler.NewUploadHandler(minioClient)

	// API路由组
	api := engine.Group("/api")
	{
		// 认证路由（无需登录）
		auth := api.Group("/auth")
		{
			auth.POST("/register", userHandler.Register)
			auth.POST("/login", userHandler.Login)
		}

		// 需要登录的路由
		protected := api.Group("")
		protected.Use(middleware.AuthMiddleware())
		{
			// 用户路由
			user := protected.Group("/user")
			{
				user.GET("/info/:id", userHandler.GetInfo)
				user.PUT("/profile", userHandler.UpdateProfile)
				user.PUT("/status", userHandler.UpdateStatus)
			}

			// 好友路由
			friend := protected.Group("/friend")
			{
				friend.GET("/list", friendHandler.GetList)
				friend.POST("/add", friendHandler.Add)
				friend.DELETE("/:friendId", friendHandler.Remove)
			}

			// 群组路由
			group := protected.Group("/group")
			{
				group.GET("/list", groupHandler.GetList)
				group.POST("/create", groupHandler.Create)
				group.POST("/:groupId/join", groupHandler.Join)
				group.DELETE("/:groupId/leave", groupHandler.Leave)
				group.GET("/:groupId/members", groupHandler.GetMembers)
				group.DELETE("/:groupId/members/:userId", groupHandler.RemoveMember)
			}

			// 消息路由
			message := protected.Group("/message")
			{
				message.GET("/private/:userId", messageHandler.GetPrivateMessages)
				message.GET("/group/:groupId", messageHandler.GetGroupMessages)
				message.POST("/read", messageHandler.MarkRead)
				message.GET("/unread", messageHandler.GetUnreadCount)
			}

			// 文件上传
			protected.POST("/upload", uploadHandler.Upload)
		}
	}

	// 健康检查
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	return &Router{engine: engine}
}

// Run 启动服务器
func (r *Router) Run(addr string) error {
	return r.engine.Run(addr)
}
