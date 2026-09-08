package mysql

import (
	"context"
	"math"
	"time"

	"penguin-chess/server/model/entity"
)

// FinishGame 事务：写对局历史 + 更新双方战绩与 Elo（K=32 标准公式）
func (d *DB) FinishGame(ctx context.Context, r *entity.GameResult, elo0, elo1 int) error {
	// 期望胜率
	e0 := 1.0 / (1.0 + math.Pow(10, float64(elo1-elo0)/400.0))
	var s0 float64 // 实际得分
	var win0, loss0, draw int
	var win1, loss1 int
	switch r.Winner {
	case 0:
		s0 = 1.0
		win0, loss1 = 1, 1
	case 1:
		s0 = 0.0
		loss0, win1 = 1, 1
	default:
		s0 = 0.5
		draw = 1
	}
	const k = 32
	delta0 := int(math.Round(k * (s0 - e0)))
	delta1 := -delta0

	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO games (room_id, player0_id, player1_id, player0_name, player1_name,
			player0_score, player1_score, winner, elo_delta, total_ply)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.RoomID, r.Player0ID, r.Player1ID, r.Player0Name, r.Player1Name,
		r.Player0Score, r.Player1Score, r.Winner, delta0, r.TotalPly,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET wins = wins + ?, losses = losses + ?, draws = draws + ?,
			elo = GREATEST(100, elo + ?) WHERE id = ?`,
		win0, loss0, draw, delta0, r.Player0ID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET wins = wins + ?, losses = losses + ?, draws = draws + ?,
			elo = GREATEST(100, elo + ?) WHERE id = ?`,
		win1, win1 == 0 && loss1 == 0 && draw == 1, 0, delta1, r.Player1ID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// RecentGames 查询玩家最近 n 局（双视角：作为 player0 或 player1）
func (d *DB) RecentGames(ctx context.Context, userID int64, n int) ([]entity.GameRecord, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT id, room_id, player0_id, player0_name, player1_name,
			player0_score, player1_score, winner, elo_delta, finished_at
		 FROM games
		 WHERE player0_id = ? OR player1_id = ?
		 ORDER BY finished_at DESC LIMIT ?`,
		userID, userID, n,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []entity.GameRecord
	for rows.Next() {
		var (
			g                       entity.GameRecord
			p0id                    int64
			p0name, p1name          string
			p0s, p1s, winner, delta int
			finished                time.Time
		)
		if err := rows.Scan(&g.ID, &g.RoomID, &p0id, &p0name, &p1name, &p0s, &p1s, &winner, &delta, &finished); err != nil {
			return nil, err
		}
		g.FinishedAt = finished
		g.EloDelta = delta
		g.Winner = winner
		if p0id == userID {
			g.Opponent = p1name
			g.MyScore, g.OppScore = p0s, p1s
		} else {
			g.Opponent = p0name
			g.MyScore, g.OppScore = p1s, p0s
			g.EloDelta = -delta
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
