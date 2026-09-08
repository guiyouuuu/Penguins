package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"penguin-chess/server/config"
	"penguin-chess/server/internal/app"
	"penguin-chess/server/internal/web"
	logging "penguin-chess/server/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("server.failed", "error", err)
		os.Exit(1)
	}
}

func run() (result error) {
	addr := flag.String("addr", "", "覆盖 HTTP 监听地址")
	file := flag.String("config", "config.yml", "YAML 配置文件；环境变量优先，空路径表示仅使用环境变量")
	flag.Parse()
	cfg, err := config.Load(*file)
	if err != nil {
		return err
	}
	if *addr != "" {
		cfg.HTTPAddr = *addr
	}
	logger, closer, err := logging.New(logging.Options{Level: cfg.LogLevel, File: cfg.LogFile, MaxSizeMB: cfg.LogMaxSizeMB, MaxBackups: cfg.LogMaxBackups, MaxAgeDays: cfg.LogMaxAgeDays, Compress: true, Secrets: cfg.Secrets()})
	if err != nil {
		return err
	}
	defer closer.Close()
	defer func() {
		if value := recover(); value != nil {
			logger.Error("server.panic", "panic", value, "stack", string(debug.Stack()))
			result = fmt.Errorf("server startup panic; see configured log file")
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, cfg, web.Files(), logger); err != nil {
		logger.Error("server.failed", "error", err)
		return fmt.Errorf("server stopped with an error; see configured log file")
	}
	return nil
}
