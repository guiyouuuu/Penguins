// Package config 读取并校验配置。环境变量优先于 YAML 文件，源码不含连接凭据。
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	driver "github.com/go-sql-driver/mysql"
)

type Config struct {
	HTTPAddr, MySQLDSN, RedisAddr, RedisPass, JWTSecret, InstanceID string
	LogLevel, LogFile                                               string
	LogMaxSizeMB, LogMaxBackups, LogMaxAgeDays                      int
	RequestTimeout, StartupTimeout, ShutdownTimeout                 time.Duration
}

func Load(configFile string) (Config, error) {
	values, err := loadYAML(configFile)
	if err != nil {
		return Config{}, err
	}
	get := func(key, fallback string) string {
		if value, ok := os.LookupEnv(key); ok {
			return value
		}
		if value, ok := values[key]; ok {
			return value
		}
		return fallback
	}
	c := Config{
		HTTPAddr: get("PENGUIN_ADDR", ":8080"), MySQLDSN: get("PENGUIN_MYSQL_DSN", ""),
		RedisAddr: get("PENGUIN_REDIS_ADDR", "127.0.0.1:6379"), RedisPass: get("PENGUIN_REDIS_PASS", ""),
		JWTSecret: get("PENGUIN_JWT_SECRET", ""), InstanceID: get("PENGUIN_INSTANCE_ID", ""),
		LogLevel: get("PENGUIN_LOG_LEVEL", "info"), LogFile: get("PENGUIN_LOG_FILE", "logs/server.log"),
	}
	for key, target := range map[string]*int{"PENGUIN_LOG_MAX_SIZE_MB": &c.LogMaxSizeMB, "PENGUIN_LOG_MAX_BACKUPS": &c.LogMaxBackups, "PENGUIN_LOG_MAX_AGE_DAYS": &c.LogMaxAgeDays} {
		fallback := "10"
		if key == "PENGUIN_LOG_MAX_BACKUPS" {
			fallback = "5"
		}
		if key == "PENGUIN_LOG_MAX_AGE_DAYS" {
			fallback = "30"
		}
		n, err := strconv.Atoi(get(key, fallback))
		if err != nil || n <= 0 {
			return c, fmt.Errorf("%s must be a positive integer", key)
		}
		*target = n
	}
	for key, target := range map[string]*time.Duration{"PENGUIN_REQUEST_TIMEOUT": &c.RequestTimeout, "PENGUIN_STARTUP_TIMEOUT": &c.StartupTimeout, "PENGUIN_SHUTDOWN_TIMEOUT": &c.ShutdownTimeout} {
		fallback := "10s"
		if key == "PENGUIN_STARTUP_TIMEOUT" {
			fallback = "15s"
		}
		d, err := time.ParseDuration(get(key, fallback))
		if err != nil || d <= 0 {
			return c, fmt.Errorf("%s must be a positive duration", key)
		}
		*target = d
	}
	if strings.TrimSpace(c.MySQLDSN) == "" {
		return c, errors.New("PENGUIN_MYSQL_DSN is required")
	}
	parsed, err := driver.ParseDSN(c.MySQLDSN)
	if err != nil {
		return c, errors.New("PENGUIN_MYSQL_DSN is invalid")
	}
	if parsed.Timeout == 0 {
		parsed.Timeout = 5 * time.Second
	}
	if parsed.ReadTimeout == 0 {
		parsed.ReadTimeout = 5 * time.Second
	}
	if parsed.WriteTimeout == 0 {
		parsed.WriteTimeout = 5 * time.Second
	}
	c.MySQLDSN = parsed.FormatDSN()
	if len(c.JWTSecret) < 32 {
		return c, errors.New("PENGUIN_JWT_SECRET must contain at least 32 bytes")
	}
	if _, _, err := net.SplitHostPort(c.HTTPAddr); err != nil {
		return c, errors.New("PENGUIN_ADDR must be host:port")
	}
	if _, _, err := net.SplitHostPort(c.RedisAddr); err != nil {
		return c, errors.New("PENGUIN_REDIS_ADDR must be host:port")
	}
	return c, nil
}

// Secrets 仅供日志脱敏，不得将返回值作为日志属性输出。
func (c Config) Secrets() []string {
	secrets := []string{c.MySQLDSN, c.RedisPass, c.JWTSecret}
	if parsed, err := driver.ParseDSN(c.MySQLDSN); err == nil {
		secrets = append(secrets, parsed.Passwd)
	}
	return secrets
}
