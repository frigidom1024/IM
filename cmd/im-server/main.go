package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gochat/internal/application"
	"gochat/internal/pkg/config"
)

func main() {
	configFile := flag.String("config", "internal/config/config-dev.yaml", "config file path")
	flag.Parse()

	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	app, err := application.New(cfg)
	if err != nil {
		log.Fatalf("初始化应用失败: %v", err)
	}
	defer app.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		log.Printf("应用运行失败: %v", err)
	}
}

