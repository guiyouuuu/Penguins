package config

import (
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type fileConfig struct {
	Server struct {
		Addr             string   `yaml:"addr"`
		InstanceID       string   `yaml:"instance_id"`
		RequestTimeout   string   `yaml:"request_timeout"`
		StartupTimeout   string   `yaml:"startup_timeout"`
		ShutdownTimeout  string   `yaml:"shutdown_timeout"`
		WSAllowedOrigins []string `yaml:"ws_allowed_origins"`
	} `yaml:"server"`
	MySQL struct {
		DSN string `yaml:"dsn"`
	} `yaml:"mysql"`
	Redis struct {
		Addr     string `yaml:"addr"`
		Password string `yaml:"password"`
	} `yaml:"redis"`
	Auth struct {
		JWTSecret string `yaml:"jwt_secret"`
	} `yaml:"auth"`
	Log struct {
		Level      string `yaml:"level"`
		File       string `yaml:"file"`
		MaxSizeMB  int    `yaml:"max_size_mb"`
		MaxBackups int    `yaml:"max_backups"`
		MaxAgeDays int    `yaml:"max_age_days"`
	} `yaml:"log"`
}

func loadYAML(path string) (map[string]string, error) {
	if path == "" {
		return map[string]string{}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open YAML configuration file; check -config path")
	}
	defer file.Close()
	var c fileConfig
	c.Server.Addr = ":8080"
	c.Server.RequestTimeout, c.Server.StartupTimeout, c.Server.ShutdownTimeout = "10s", "15s", "10s"
	c.Redis.Addr = "127.0.0.1:6379"
	c.Log.Level, c.Log.File = "info", "logs/server.log"
	c.Log.MaxSizeMB, c.Log.MaxBackups, c.Log.MaxAgeDays = 10, 5, 30
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&c); err != nil {
		// YAML 类型错误可能包含原始值，不能将解析器错误直接输出到日志。
		return nil, errors.New("invalid YAML configuration; check field names, indentation and value types")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("YAML configuration must contain exactly one document")
	}
	return map[string]string{
		"PENGUIN_ADDR": c.Server.Addr, "PENGUIN_INSTANCE_ID": c.Server.InstanceID,
		"PENGUIN_REQUEST_TIMEOUT": c.Server.RequestTimeout, "PENGUIN_STARTUP_TIMEOUT": c.Server.StartupTimeout,
		"PENGUIN_SHUTDOWN_TIMEOUT":   c.Server.ShutdownTimeout,
		"PENGUIN_WS_ALLOWED_ORIGINS": strings.Join(c.Server.WSAllowedOrigins, ","),
		"PENGUIN_MYSQL_DSN":          c.MySQL.DSN, "PENGUIN_REDIS_ADDR": c.Redis.Addr,
		"PENGUIN_REDIS_PASS": c.Redis.Password, "PENGUIN_JWT_SECRET": c.Auth.JWTSecret,
		"PENGUIN_LOG_LEVEL": c.Log.Level, "PENGUIN_LOG_FILE": c.Log.File,
		"PENGUIN_LOG_MAX_SIZE_MB":  strconv.Itoa(c.Log.MaxSizeMB),
		"PENGUIN_LOG_MAX_BACKUPS":  strconv.Itoa(c.Log.MaxBackups),
		"PENGUIN_LOG_MAX_AGE_DAYS": strconv.Itoa(c.Log.MaxAgeDays),
	}, nil
}
