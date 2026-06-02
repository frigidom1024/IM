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
	"gochat/pkg/minio"
	"gochat/pkg/mysql"
	"gochat/pkg/snowflake"

	"go.uber.org/zap"
)

var (
	configFile = flag.String("config", "config/config-dev.yaml", "config file path")
)

func main() {
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化日志
	if err := logger.Init(cfg.Logger.Level, cfg.Logger.Output); err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	defer logger.Sync()

	// 初始化MySQL
	if err := mysql.Init(&cfg.MySQL); err != nil {
		log.Fatalf("初始化MySQL失败: %v", err)
	}
	defer mysql.Close()

	// 初始化雪花ID生成器
	if err := snowflake.Init(1, 1); err != nil {
		log.Fatalf("初始化雪花ID生成器失败: %v", err)
	}

	// 初始化 MinIO 客户端
	minioClient, err := minio.NewClient(&cfg.MinIO)
	if err != nil {
		log.Fatalf("初始化 MinIO 失败: %v", err)
	}
	logger.Info("MinIO 客户端初始化成功", zap.String("bucket", cfg.MinIO.Bucket))

	// 创建仓储
	userRepo := registry.NewUserRepository()
	friendRepo := registry.NewFriendRepository()
	groupRepo := registry.NewGroupRepository()
	groupMemberRepo := registry.NewGroupMemberRepository()
	messageRepo := registry.NewMessageRepository()
	messageReadRepo := registry.NewMessageReadRepository()

	// 创建服务
	messageService := service.NewMessageService(messageRepo, messageReadRepo)
	groupService := service.NewGroupService(groupRepo, groupMemberRepo)

	// 创建并启动API服务器
	router := api.NewRouter(
		userRepo,
		friendRepo,
		groupRepo,
		groupMemberRepo,
		messageRepo,
		messageReadRepo,
		minioClient,
	)

	// 启动WebSocket服务器
	wsServer := NewWsServer(messageService, groupService)

	// 启动API服务器（阻塞）
	go func() {
		addr := cfg.Server.APIAddr
		logger.Info("启动API服务器", zap.String("addr", addr))
		if err := router.Run(addr); err != nil {
			log.Fatalf("启动API服务器失败: %v", err)
		}
	}()

	// 启动WebSocket服务器
	go func() {
		addr := cfg.Server.ConnectAddr
		logger.Info("启动WebSocket服务器", zap.String("addr", addr))
		if err := wsServer.Run(addr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("启动WebSocket服务器失败: %v", err)
		}
	}()

	logger.Info("所有服务启动完成")

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("正在关闭...")
}
