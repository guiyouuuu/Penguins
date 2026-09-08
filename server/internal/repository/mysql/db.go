package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"penguin-chess/server/pkg/database"
)

// DB MySQL 封装
type DB struct {
	sql *sql.DB
}

// Open 打开连接池并自动迁移表结构
func Open(ctx context.Context, dsn string) (*DB, error) {
	db, err := database.Open(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 MySQL: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("迁移失败: %w", err)
	}
	return &DB{sql: db}, nil
}

// Close 关闭连接池
func (d *DB) Close() error { return d.sql.Close() }

func (d *DB) Ping(ctx context.Context) error { return d.sql.PingContext(ctx) }
