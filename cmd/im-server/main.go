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
	if err := run(); err != nil {
		log.Printf("应用失败: %v", err)
		os.Exit(1)
	}
}

func run() error {
	configFile := flag.String("config", "internal/config/config-dev.yaml", "config file path")
	flag.Parse()

	cfg, err := config.Load(*configFile)
	if err != nil {
		return err
	}

	app, err := application.New(cfg)
	if err != nil {
		return err
	}
	defer app.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return app.Run(ctx)
}
