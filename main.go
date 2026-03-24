package main

import (
	"flag"
	"log"

	"gochat/api"
	"gochat/logic/registry"
	"gochat/pkg/config"
	"gochat/pkg/logger"
	"gochat/pkg/mysql"

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
	if err := logger.Init("info", "stdout"); err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	defer logger.Sync()

	// 初始化MySQL
	if err := mysql.Init(&cfg.MySQL); err != nil {
		log.Fatalf("初始化MySQL失败: %v", err)
	}
	defer mysql.Close()

	// 创建仓储
	userRepo := registry.NewUserRepository()
	friendRepo := registry.NewFriendRepository()
	groupRepo := registry.NewGroupRepository()
	groupMemberRepo := registry.NewGroupMemberRepository()
	messageRepo := registry.NewMessageRepository()
	messageReadRepo := registry.NewMessageReadRepository()

	// 创建并启动API服务器
	router := api.NewRouter(
		userRepo,
		friendRepo,
		groupRepo,
		groupMemberRepo,
		messageRepo,
		messageReadRepo,
	)

	addr := cfg.Server.APIAddr
	logger.Info("启动API服务器", zap.String("addr", addr))
	if err := router.Run(addr); err != nil {
		log.Fatalf("启动服务器失败: %v", err)
	}
}
