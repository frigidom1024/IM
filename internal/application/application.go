package application

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	clientv3 "go.etcd.io/etcd/client/v3"
	"gochat/internal/api"
	"gochat/internal/connect"
	"gochat/internal/logic/registry"
	"gochat/internal/logic/service"
	"gochat/internal/pkg/config"
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/minio"
	"gochat/internal/pkg/mysql"
	"gochat/internal/pkg/snowflake"
	"gochat/internal/pkg/telemetry"
	"gochat/internal/pkg/util"

	"go.uber.org/zap"
)

// Application 负责组装并运行 GoChat 的基础设施、HTTP API 和 WebSocket 服务。
type Application struct {
	cfg         *config.Config
	apiServer   *api.Router
	wsServer    *connect.WsConnServer
	ready       atomic.Bool
	closeOnce   sync.Once
	redisClient *redis.Client
	etcdClient  *clientv3.Client
}

// New 创建应用并初始化应用运行所需的基础设施。
func New(cfg *config.Config) (*Application, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置校验失败: %w", err)
	}

	util.Init(cfg.JWT.Secret)
	if err := logger.Init(cfg.Logger.Level, cfg.Logger.Output); err != nil {
		return nil, fmt.Errorf("初始化日志失败: %w", err)
	}

	cleanup := func() {
		_ = mysql.Close()
		_ = logger.Sync()
	}

	if err := mysql.Init(&cfg.MySQL); err != nil {
		_ = logger.Sync()
		return nil, fmt.Errorf("初始化MySQL失败: %w", err)
	}

	if err := snowflake.Init(1, 1); err != nil {
		cleanup()
		return nil, fmt.Errorf("初始化雪花ID生成器失败: %w", err)
	}

	minioClient, err := minio.NewClient(&cfg.MinIO)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("初始化 MinIO 失败: %w", err)
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

	app := &Application{cfg: cfg}
	sqlDB, err := mysql.Client.DB()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("get database pool: %w", err)
	}
	telemetry.Default.SetDBStats(sqlDB.Stats)
	checks := map[string]api.DependencyCheck{"mysql": sqlDB.PingContext, "minio": minioClient.CheckReady, "redis": nil, "etcd": nil}
	if cfg.Redis.Enabled {
		app.redisClient = redis.NewClient(&redis.Options{Addr: fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port), Password: cfg.Redis.Password, DB: cfg.Redis.DB, DialTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, ContextTimeoutEnabled: true})
		checks["redis"] = func(ctx context.Context) error { return app.redisClient.Ping(ctx).Err() }
		telemetry.Default.SetRedisStats(func() telemetry.RedisStats {
			stats := app.redisClient.PoolStats()
			return telemetry.RedisStats{Total: stats.TotalConns, Idle: stats.IdleConns, Hits: stats.Hits, Misses: stats.Misses, Timeouts: stats.Timeouts}
		})
	}
	if cfg.Etcd.Enabled {
		app.etcdClient, err = clientv3.New(clientv3.Config{Endpoints: cfg.Etcd.Endpoints, DialTimeout: 2 * time.Second})
		if err != nil {
			app.Close()
			return nil, fmt.Errorf("initialize etcd: %w", err)
		}
		checks["etcd"] = func(ctx context.Context) error {
			_, err := app.etcdClient.Get(ctx, "/gochat/readiness", clientv3.WithLimit(1))
			return err
		}
	}
	app.apiServer = api.NewRouter(
		userRepo,
		friendRepo,
		groupRepo,
		groupMemberRepo,
		messageRepo,
		messageReadRepo,
		minioClient,
		cfg.Auth.Mode,
		cfg.Auth.ExternalUserAPI,
		cfg.Auth.RateLimitPerMinute,
		cfg.Auth.ExternalTimeoutSeconds,
		cfg.CORS.AllowedOrigins,
		app.IsReady,
		api.WithDependencyChecks(2*time.Second, checks),
	)
	app.wsServer = newWebSocketServer(messageService, groupService)
	return app, nil
}

// IsReady reports whether the application has completed startup and accepts traffic.
func (a *Application) IsReady() bool {
	return a != nil && a.ready.Load()
}

// Run 启动 API 和 WebSocket 服务，直到收到停止信号或任一服务退出。
func (a *Application) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	errCh := make(chan error, 2)
	apiReady := make(chan struct{})
	wsReady := make(chan struct{})

	go func() {
		err := a.apiServer.RunReady(ctx, a.cfg.Server.APIAddr, apiReady)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("API服务器退出: %w", err)
			return
		}
		errCh <- nil
	}()

	go func() {
		err := a.wsServer.RunReady(ctx, a.cfg.Server.ConnectAddr, wsReady)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("WebSocket服务器退出: %w", err)
			return
		}
		errCh <- nil
	}()

	readyCount := 0
	apiReadyCh, wsReadyCh := (<-chan struct{})(apiReady), (<-chan struct{})(wsReady)
	for readyCount < 2 {
		select {
		case <-apiReadyCh:
			readyCount++
			apiReadyCh = nil
		case <-wsReadyCh:
			readyCount++
			wsReadyCh = nil
		case err := <-errCh:
			a.ready.Store(false)
			_ = a.shutdown()
			return err
		case <-ctx.Done():
			a.ready.Store(false)
			return a.shutdown()
		}
	}

	a.ready.Store(true)
	logger.Info("所有服务启动完成")

	select {
	case <-ctx.Done():
		a.ready.Store(false)
		return a.shutdown()
	case err := <-errCh:
		a.ready.Store(false)
		return err
	}
}

func (a *Application) shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	apiErr := a.apiServer.Shutdown(shutdownCtx)
	wsErr := a.wsServer.Shutdown(shutdownCtx)
	if apiErr != nil {
		return fmt.Errorf("关闭API服务器失败: %w", apiErr)
	}
	if wsErr != nil {
		return fmt.Errorf("关闭WebSocket服务器失败: %w", wsErr)
	}
	logger.Info("服务已优雅关闭")
	return nil
}

// Close 释放应用初始化的基础设施资源。
func (a *Application) Close() {
	a.closeOnce.Do(func() {
		a.ready.Store(false)
		if a.redisClient != nil {
			_ = a.redisClient.Close()
		}
		if a.etcdClient != nil {
			_ = a.etcdClient.Close()
		}
		telemetry.Default.SetDBStats(nil)
		_ = mysql.Close()
		_ = logger.Sync()
	})
}
