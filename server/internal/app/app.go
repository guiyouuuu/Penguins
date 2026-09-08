// Package app 负责依赖装配、启动检查和优雅关闭。
package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"penguin-chess/server/api/handler"
	"penguin-chess/server/api/router"
	"penguin-chess/server/config"
	"penguin-chess/server/internal/repository/mysql"
	"penguin-chess/server/internal/room"
	"penguin-chess/server/internal/service"
	"penguin-chess/server/pkg/auth"
)

func Run(ctx context.Context, cfg config.Config, assets fs.FS, logger *slog.Logger) error {
	instanceID := cfg.InstanceID
	if instanceID == "" {
		instanceID = uuid.NewString()[:8]
	}
	logger = logger.With("service", "penguin-chess", "instance_id", instanceID)
	startup, cancel := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancel()
	db, err := mysql.Open(startup, cfg.MySQLDSN)
	if err != nil {
		return fmt.Errorf("initialize mysql: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("mysql.close_failed", "error", err)
		}
	}()
	logger.Info("mysql.ready")
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPass, DialTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, ContextTimeoutEnabled: true})
	defer func() {
		if err := rdb.Close(); err != nil {
			logger.Error("redis.close_failed", "error", err)
		}
	}()
	if err := rdb.Ping(startup).Err(); err != nil {
		return fmt.Errorf("initialize redis: %w", err)
	}
	logger.Info("redis.ready")
	authSvc := auth.New(cfg.JWTSecret)
	// 收到退出信号后先处理在途请求和连接，再关闭依赖上下文。
	workCtx, stopWork := context.WithCancel(context.Background())
	defer stopWork()
	hub := room.NewHub(workCtx, rdb, db, logger, instanceID)
	var shutdownDeadline time.Time
	defer func() {
		if shutdownDeadline.IsZero() {
			shutdownDeadline = time.Now().Add(cfg.ShutdownTimeout)
		}
		cleanup, cancel := context.WithDeadline(context.Background(), shutdownDeadline)
		defer cancel()
		if err := hub.Close(cleanup); err != nil {
			logger.Error("room.shutdown_failed", "error", err)
		}
	}()
	httpHandler := router.New(handler.Options{Accounts: service.NewAccount(db, authSvc), Auth: authSvc, Hub: hub, Static: assets, Logger: logger, RequestTimeout: cfg.RequestTimeout, Ready: func(ctx context.Context) error {
		if err := db.Ping(ctx); err != nil {
			return fmt.Errorf("mysql readiness: %w", err)
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			return fmt.Errorf("redis readiness: %w", err)
		}
		return nil
	}})
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	server := &http.Server{Handler: httpHandler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError)}
	results := make(chan error, 1)
	go func() {
		defer func() {
			if value := recover(); value != nil {
				logger.Error("server.serve_panic", "panic", value, "stack", string(debug.Stack()))
				results <- errors.New("http serve panic")
			}
		}()
		results <- server.Serve(listener)
	}()
	logger.Info("server.started", "address", listener.Addr().String(), "log_file", cfg.LogFile)
	select {
	case err := <-results:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		logger.Info("server.stopping")
		shutdownDeadline = time.Now().Add(cfg.ShutdownTimeout)
		shutdown, cancel := context.WithDeadline(context.Background(), shutdownDeadline)
		defer cancel()
		err := server.Shutdown(shutdown)
		if err != nil {
			_ = server.Close()
		}
		if serveErr := <-results; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			err = errors.Join(err, serveErr)
		}
		logger.Info("server.http_stopped")
		return err
	}
}
