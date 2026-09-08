package room

import (
	"runtime/debug"
	"time"

	"github.com/google/uuid"

	"penguin-chess/server/internal/game"
	apperr "penguin-chess/server/pkg/errors"
)

// playerMove 玩家走子（服务器权威：锁 → 校验 → 应用 → 广播）
func (h *Hub) playerMove(c *Client, mv *game.Move) {
	roomID := c.room()
	if roomID == "" || c.seatIndex() < 0 {
		sendError(c, apperr.NotInRoom)
		return
	}
	seat := c.seatIndex()
	mv.Player = seat
	err := h.rs.withLock(roomID, func() error {
		meta, err := h.rs.GetMeta(roomID)
		if err != nil {
			return err
		}
		if meta == nil {
			return errRoomGone
		}
		if !meta.Started {
			return apperr.BadPhase
		}
		state, err := h.rs.GetState(roomID)
		if err != nil {
			return err
		}
		if state == nil {
			return errRoomGone
		}
		if state.Phase == game.PhaseFinished {
			return game.ErrGameOver
		}
		// 席位校验：pvp 严格匹配用户；AI 房玩家固定 seat 0
		if meta.Mode == "pvp" {
			seatUID := meta.Seat0UID
			if seat == 1 {
				seatUID = meta.Seat1UID
			}
			if c.userID <= 0 || seatUID != c.userID {
				return errNotSeatOwner
			}
		} else if seat != 0 {
			return errNotSeatOwner
		}
		next, err := state.ApplyMove(mv)
		if err != nil {
			return err
		}
		if err := h.rs.saveState(roomID, next); err != nil {
			return err
		}
		c.logger.Info("game.move_applied", "room_id", roomID, "kind", mv.Kind, "from", mv.From, "to", mv.To, "ply", next.Ply, "next_turn", next.Turn)
		if next.Phase == game.PhaseFinished {
			SettleGame(h.ctx, h.logger, h.db, roomID, meta, next)
			h.publishRoom(roomID, msgOver{Type: "over", State: next, Winner: next.Winner, Last: mv})
		} else {
			h.publishRoom(roomID, msgState{Type: "state", State: next, Last: mv})
		}
		// AI 房间轮到 AI → 锁外触发
		if meta.Mode == "ai" && next.Phase != game.PhaseFinished && next.Turn == 1 {
			h.run("ai.move", func() { h.aiMove(roomID) })
		}
		return nil
	})
	if err != nil {
		if err == errRoomGone {
			sendError(c, apperr.RoomGone)
		} else if err == errNotSeatOwner {
			sendError(c, apperr.Forbidden)
		} else {
			sendError(c, err)
		}
	}
}

// aiMove AI 行动（可能连续多步直到轮到玩家或终局）
func (h *Hub) aiMove(roomID string) {
	defer func() {
		if value := recover(); value != nil {
			h.logger.Error("ai.panic", "room_id", roomID, "panic", value, "stack", string(debug.Stack()))
			h.aiError(roomID)
		}
	}()
	if !waitFor(h.ctx, 650*time.Millisecond) {
		return
	}
	for {
		done := true
		err := h.rs.withLock(roomID, func() error {
			state, err := h.rs.GetState(roomID)
			if err != nil {
				return err
			}
			if state == nil || state.Phase == game.PhaseFinished {
				return nil
			}
			if state.Turn != 1 {
				return nil
			}
			mv := aiChoose(state)
			if mv == nil {
				return nil
			}
			next, err := state.ApplyMove(mv)
			if err != nil {
				return err
			}
			if err := h.rs.saveState(roomID, next); err != nil {
				return err
			}
			h.logger.Info("game.ai_move_applied", "room_id", roomID, "kind", mv.Kind, "from", mv.From, "to", mv.To, "ply", next.Ply, "next_turn", next.Turn)
			if next.Phase == game.PhaseFinished {
				h.publishRoom(roomID, msgOver{Type: "over", State: next, Winner: next.Winner, Last: mv})
			} else {
				h.publishRoom(roomID, msgState{Type: "state", State: next, Last: mv})
			}
			if next.Turn == 1 {
				done = false // AI 还要继续（玩家已被困毙跳过）
			}
			return nil
		})
		if err != nil {
			h.logger.Error("ai.move_failed", "room_id", roomID, "error", err)
			h.aiError(roomID)
			return
		}
		if done {
			return
		}
		if !waitFor(h.ctx, 650*time.Millisecond) {
			return
		}
	}
}

func (h *Hub) resign(c *Client) {
	roomID := c.room()
	if roomID == "" || c.seatIndex() < 0 {
		return
	}
	seat := c.seatIndex()
	err := h.rs.withLock(roomID, func() error {
		meta, err := h.rs.GetMeta(roomID)
		if err != nil {
			return err
		}
		if meta == nil {
			return errRoomGone
		}
		if !meta.Started {
			return apperr.BadPhase
		}
		state, err := h.rs.GetState(roomID)
		if err != nil {
			return err
		}
		if state == nil {
			return errRoomGone
		}
		if state.Phase == game.PhaseFinished {
			return nil
		}
		state.Phase = game.PhaseFinished
		state.Winner = 1 - seat
		if err := h.rs.saveState(roomID, state); err != nil {
			return err
		}
		SettleGame(h.ctx, h.logger, h.db, roomID, meta, state)
		h.publishRoom(roomID, msgOver{Type: "over", State: state, Winner: state.Winner, Last: nil})
		return nil
	})
	if err != nil {
		sendError(c, err)
	}
}

func (h *Hub) rematch(c *Client) {
	roomID := c.room()
	if roomID == "" || c.seatIndex() < 0 {
		return
	}
	seat := c.seatIndex()
	err := h.rs.withLock(roomID, func() error {
		meta, err := h.rs.GetMeta(roomID)
		if err != nil {
			return err
		}
		if meta == nil {
			return errRoomGone
		}
		state, err := h.rs.GetState(roomID)
		if err != nil {
			return err
		}
		if !meta.Started || state == nil || state.Phase != game.PhaseFinished {
			return apperr.BadPhase
		}
		if meta.Mode == "ai" {
			// AI 房直接重开
			state, err := h.rs.ResetForRematch(roomID, meta)
			if err != nil {
				return err
			}
			if state == nil {
				return errRoomGone
			}
			names := [2]string{state.Players[0].Name, state.Players[1].Name}
			h.publishRoom(roomID, msgStart{Type: "start", State: state, Names: names})
			return nil
		}
		// pvp：双确认
		key := metaKey(roomID)
		field := "rematch0"
		if seat == 1 {
			field = "rematch1"
		}
		pipe := h.rs.rdb.TxPipeline()
		pipe.HSet(h.ctx, key, field, "1")
		pipe.Expire(h.ctx, key, roomTTL)
		if _, err := pipe.Exec(h.ctx); err != nil {
			return err
		}
		vals, err := h.rs.rdb.HMGet(h.ctx, key, "rematch0", "rematch1").Result()
		if err != nil {
			return err
		}
		r0 := vals[0] != nil && vals[0].(string) == "1"
		r1 := vals[1] != nil && vals[1].(string) == "1"
		if r0 && r1 {
			// 双方就绪 → 新局
			if err := h.rs.rdb.HDel(h.ctx, key, "rematch0", "rematch1").Err(); err != nil {
				return err
			}
			state, err := h.rs.ResetForRematch(roomID, meta)
			if err != nil {
				return err
			}
			names := [2]string{state.Players[0].Name, state.Players[1].Name}
			h.publishRoom(roomID, msgStart{Type: "start", State: state, Names: names})
		} else {
			// 通知对方
			h.publishRoom(roomID, msgSimple{Type: "rematch_ask"})
		}
		return nil
	})
	if err != nil {
		sendError(c, err)
	}
}

func (h *Hub) aiError(roomID string) {
	h.publishRoom(roomID, msgError{Type: "error", Code: apperr.Internal.Code, Message: apperr.Internal.Message, Msg: apperr.Internal.Message, RequestID: uuid.NewString()})
}
