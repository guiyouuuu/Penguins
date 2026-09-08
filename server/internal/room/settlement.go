package room

import (
	"context"
	"log/slog"
	"time"

	"penguin-chess/server/model/entity"

	"penguin-chess/server/internal/game"
	"penguin-chess/server/internal/repository/mysql"
)

// ---------- 结算（对局结束写 MySQL） ----------

// SettleGame pvp 双登录局写库（游客局跳过）
func SettleGame(ctx context.Context, logger *slog.Logger, db *mysql.DB, roomID string, meta *RoomMeta, st *game.GameState) {
	if db == nil || meta.Mode != "pvp" || meta.Seat0UID <= 0 || meta.Seat1UID <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// 读取双方赛前 Elo
	u0, err0 := db.GetUserByID(ctx, meta.Seat0UID)
	u1, err1 := db.GetUserByID(ctx, meta.Seat1UID)
	if err0 != nil || err1 != nil || u0 == nil || u1 == nil {
		logger.Error("game.settlement_users_failed", "room_id", roomID, "player0_error", err0, "player1_error", err1)
		return
	}
	r := &entity.GameResult{
		RoomID:       roomID,
		Player0ID:    meta.Seat0UID,
		Player1ID:    meta.Seat1UID,
		Player0Name:  meta.Seat0Name,
		Player1Name:  meta.Seat1Name,
		Player0Score: st.Players[0].Score,
		Player1Score: st.Players[1].Score,
		Winner:       st.Winner,
		TotalPly:     st.Ply,
	}
	if err := db.FinishGame(ctx, r, u0.Elo, u1.Elo); err != nil {
		logger.Error("game.settlement_failed", "room_id", roomID, "error", err)
		return
	}
	logger.Info("game.settled", "room_id", roomID, "winner", st.Winner, "scores", []int{st.Players[0].Score, st.Players[1].Score})
}
