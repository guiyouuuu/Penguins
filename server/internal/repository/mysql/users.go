package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"penguin-chess/server/model/entity"

	driver "github.com/go-sql-driver/mysql"
)

var ErrDuplicateUsername = errors.New("duplicate username")

// CreateUser 注册新用户（密码已 bcrypt）
func (d *DB) CreateUser(ctx context.Context, username, passwordHash string) (int64, error) {
	res, err := d.sql.ExecContext(ctx,
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		username, passwordHash,
	)
	if err != nil {
		var duplicate *driver.MySQLError
		if errors.As(err, &duplicate) && duplicate.Number == 1062 {
			return 0, fmt.Errorf("%w: %w", ErrDuplicateUsername, err)
		}
		return 0, err
	}
	return res.LastInsertId()
}

// GetUserByName 按用户名查询（返回用户与密码哈希；未找到返回 nil, nil, nil）
func (d *DB) GetUserByName(ctx context.Context, username string) (*entity.User, string, error) {
	row := d.sql.QueryRowContext(ctx,
		`SELECT id, username, password_hash, wins, losses, draws, elo FROM users WHERE username = ?`,
		username,
	)
	var u entity.User
	var hash string
	if err := row.Scan(&u.ID, &u.Username, &hash, &u.Wins, &u.Losses, &u.Draws, &u.Elo); err != nil {
		if err == sql.ErrNoRows {
			return nil, "", nil
		}
		return nil, "", err
	}
	return &u, hash, nil
}

// GetUserByID 按 ID 查询
func (d *DB) GetUserByID(ctx context.Context, id int64) (*entity.User, error) {
	row := d.sql.QueryRowContext(ctx,
		`SELECT id, username, wins, losses, draws, elo FROM users WHERE id = ?`, id,
	)
	var u entity.User
	if err := row.Scan(&u.ID, &u.Username, &u.Wins, &u.Losses, &u.Draws, &u.Elo); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// TopPlayers Elo 天梯榜
func (d *DB) TopPlayers(ctx context.Context, n int) ([]entity.User, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT id, username, wins, losses, draws, elo
		 FROM users ORDER BY elo DESC, wins DESC LIMIT ?`, n,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entity.User
	for rows.Next() {
		var u entity.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Wins, &u.Losses, &u.Draws, &u.Elo); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
