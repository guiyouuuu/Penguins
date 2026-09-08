// conntest 使用服务配置检查 MySQL / Redis 连接，不写入业务数据。
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/redis/go-redis/v9"

	"penguin-chess/server/config"
	"penguin-chess/server/pkg/database"
	logging "penguin-chess/server/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		slog.Error("connection_check.failed", "error", err)
		os.Exit(1)
	}
}
func run() error {
	file := flag.String("config", "config.yml", "YAML 配置文件")
	flag.Parse()
	cfg, err := config.Load(*file)
	if err != nil {
		return err
	}
	logger, closer, err := logging.New(logging.Options{File: cfg.LogFile, Level: cfg.LogLevel, Secrets: cfg.Secrets()})
	if err != nil {
		return err
	}
	defer closer.Close()
	// 标准日志也使用脱敏 handler，避免错误路径打印连接凭据。
	slog.SetDefault(logger)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.StartupTimeout)
	defer cancel()
	db, err := database.Open(ctx, cfg.MySQLDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	logger.Info("mysql.connection_ok")
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPass, ContextTimeoutEnabled: true})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return err
	}
	logger.Info("redis.connection_ok")
	return nil
}
