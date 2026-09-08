package mysql

import (
	"context"
	"database/sql"
	"fmt"
)

// migrate 自动建表（幂等）
func migrate(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			username      VARCHAR(32)  NOT NULL UNIQUE,
			password_hash VARCHAR(100) NOT NULL,
			wins          INT NOT NULL DEFAULT 0,
			losses        INT NOT NULL DEFAULT 0,
			draws         INT NOT NULL DEFAULT 0,
			elo           INT NOT NULL DEFAULT 1000,
			created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_elo (elo DESC)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS games (
			id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			room_id        VARCHAR(8) NOT NULL,
			player0_id     BIGINT UNSIGNED NOT NULL,
			player1_id     BIGINT UNSIGNED NOT NULL,
			player0_name   VARCHAR(32) NOT NULL,
			player1_name   VARCHAR(32) NOT NULL,
			player0_score  INT NOT NULL DEFAULT 0,
			player1_score  INT NOT NULL DEFAULT 0,
			winner         TINYINT NOT NULL,
			elo_delta      INT NOT NULL DEFAULT 0,
			total_ply      INT NOT NULL DEFAULT 0,
			finished_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_p0 (player0_id, finished_at DESC),
			INDEX idx_p1 (player1_id, finished_at DESC)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("建表: %w", err)
		}
	}
	return nil
}
