// 快速匹配：Redis ZSET 队列 + 配对锁（跨实例安全）。
package room

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	apperr "penguin-chess/server/pkg/errors"

	"github.com/redis/go-redis/v9"

	"penguin-chess/server/internal/ai"
	"penguin-chess/server/internal/game"
)

// queueEntry 匹配队列元素
type queueEntry struct {
	ConnID     string `json:"conn_id"`
	InstanceID string `json:"instance_id"`
	UID        int64  `json:"uid"`
	Name       string `json:"name"`
}

// quickMatch 快速匹配：配对成功创建房间并跨实例通知；否则入队等待并延迟自查
func (h *Hub) quickMatch(c *Client) {
	if c.userID <= 0 {
		sendError(c, apperr.Unauthenticated)
		return
	}
	if c.room() != "" {
		sendError(c, apperr.AlreadyInRoom)
		return
	}
	me := queueEntry{ConnID: c.connID, InstanceID: h.instanceID, UID: c.userID, Name: c.username}

	// 抢配对锁（短重试缓解跨实例/慢网络的锁持有窗口）
	paired := false
	for attempt := 0; attempt < 6; attempt++ {
		token := uuid.NewString()
		ok, err := h.rs.rdb.SetNX(c.ctx, keyMatchLock, token, matchLockTTL).Result()
		if err != nil {
			sendError(c, err)
			return
		}
		if !ok {
			if !waitFor(c.ctx, 30*time.Millisecond) {
				return
			}
			continue
		}
		paired, err = h.tryPair(c, me)
		h.releaseMatchLock(token)
		if err != nil {
			sendError(c, err)
			return
		}
		break
	}
	if paired {
		return
	}
	if err := h.enqueue(me); err != nil {
		sendError(c, err)
		return
	}
	c.writeJSON(msgWaiting{Type: "waiting"})
	// 延迟自查：若队列中还有其他等待者则主动配对（防止锁竞态导致双方永久等待）
	h.run("match.recheck", func() { h.recheckMatch(c, me) })
}

// recheckMatch 入队后延迟自查配对
func (h *Hub) recheckMatch(c *Client, me queueEntry) {
	for i := 0; i < 3; i++ {
		if !waitFor(c.ctx, 600*time.Millisecond) {
			return
		}
		if c.room() != "" {
			return // 已被他人配走
		}
		h.mu.RLock()
		_, alive := h.conns[c.connID]
		h.mu.RUnlock()
		if !alive {
			return // 已断线，队列由 onDisconnect 清理
		}
		// 先摘出自己的条目（避免配对竞态）；摘不到说明正被他人配对
		if !h.removeQueueEntry(me) {
			continue
		}
		token := uuid.NewString()
		ok, err := h.rs.rdb.SetNX(c.ctx, keyMatchLock, token, matchLockTTL).Result()
		if err != nil {
			sendError(c, err)
			return
		}
		if !ok {
			if err := h.enqueue(me); err != nil {
				sendError(c, err)
				return
			}
			continue
		}
		paired, err := h.tryPair(c, me)
		h.releaseMatchLock(token)
		if err != nil {
			sendError(c, err)
			return
		}
		if !paired {
			if err := h.enqueue(me); err != nil {
				sendError(c, err)
				return
			}
			continue
		}
		return
	}
}

// removeQueueEntry 摘除自己的队列条目，返回是否摘到
func (h *Hub) removeQueueEntry(e queueEntry) bool {
	data, _ := json.Marshal(e)
	n, err := h.rs.rdb.ZRem(h.ctx, keyMatchQueue, string(data)).Result()
	if err != nil {
		h.logger.Error("match.dequeue_failed", "connection_id", e.ConnID, "error", err)
		return false
	}
	return n > 0
}

// enqueue 入队（member 序列化，score 为时间戳）
func (h *Hub) enqueue(e queueEntry) error {
	data, _ := json.Marshal(e)
	if err := h.rs.rdb.ZAdd(h.ctx, keyMatchQueue, redis.Z{Score: float64(time.Now().UnixMilli()), Member: string(data)}).Err(); err != nil {
		h.logger.Error("match.enqueue_failed", "connection_id", e.ConnID, "error", err)
		return err
	}
	return nil
}

func (h *Hub) releaseMatchLock(token string) {
	if err := h.rs.rdb.Eval(h.ctx, `
		if redis.call('GET', KEYS[1]) == ARGV[1] then
			return redis.call('DEL', KEYS[1])
		end
		return 0
	`, []string{keyMatchLock}, token).Err(); err != nil {
		h.logger.Error("match.unlock_failed", "error", err)
	}
}

// tryPair 配对：找到候选则创建房间、绑定双方并广播开局
func (h *Hub) tryPair(c *Client, me queueEntry) (bool, error) {
	entries, err := h.rs.rdb.ZRange(h.ctx, keyMatchQueue, 0, -1).Result()
	if err != nil {
		return false, err
	}
	h.logger.Debug("match.search", "connection_id", c.connID, "queue_size", len(entries))
	for _, raw := range entries {
		var cand queueEntry
		if err := json.Unmarshal([]byte(raw), &cand); err != nil {
			h.rs.rdb.ZRem(h.ctx, keyMatchQueue, raw) // 清理脏数据
			continue
		}
		if cand.UID == me.UID {
			continue // 自己（旧会话残留）
		}
		removed, err := h.rs.rdb.ZRem(h.ctx, keyMatchQueue, raw).Result()
		if err != nil {
			return false, err
		}
		if removed == 0 {
			continue // 被并发取走
		}
		// 创建房间：我坐 seat0，候选坐 seat1
		code, err := h.rs.CreateRoom("pvp", me.UID, me.Name)
		if err != nil {
			return false, err
		}
		if err := h.rs.JoinSeat1(code, cand.UID, cand.Name); err != nil {
			return false, err
		}
		// 对局状态用双方真实名字
		state, err := h.rs.GetState(code)
		if err != nil {
			return false, err
		}
		if state == nil {
			return false, errRoomGone
		}
		state.Players[1].Name = cand.Name
		state.Players[1].UserID = cand.UID
		if err := h.rs.saveState(code, state); err != nil {
			return false, err
		}
		// 我方本地绑定 seat0 并告知房间号
		h.bindLocal(c, code, 0)
		c.writeJSON(msgRoom{Type: "room", Room: code, You: 0})
		// 候选方：跨实例绑定 seat1 并告知房间号
		roomMsg1, _ := json.Marshal(msgRoom{Type: "room", Room: code, You: 1})
		if err := h.publishRaw(BroadCastMsg{RoomID: code, TargetConn: cand.ConnID, BindSeat: 1, LeaveSeat: -1, Msg: roomMsg1}); err != nil {
			return false, err
		}
		// 双方绑定完成后广播开局（同频道消息按序投递）
		names := [2]string{me.Name, cand.Name}
		startMsg, _ := json.Marshal(msgStart{Type: "start", State: state, Names: names})
		if err := h.publishRaw(BroadCastMsg{RoomID: code, BindSeat: -1, LeaveSeat: -1, Msg: startMsg}); err != nil {
			return false, err
		}
		h.logger.Info("match.paired", "room_id", code, "player0_id", me.UID, "player1_id", cand.UID)
		return true, nil
	}
	return false, nil
}

// publishRaw 发布原始广播消息
func (h *Hub) publishRaw(bm BroadCastMsg) error {
	return h.publishRawContext(h.ctx, bm)
}

func (h *Hub) publishRawContext(ctx context.Context, bm BroadCastMsg) error {
	payload, err := json.Marshal(bm)
	if err != nil {
		h.logger.Error("room.broadcast_encode_failed", "room_id", bm.RoomID, "error", err)
		return err
	}
	if err := h.rs.rdb.Publish(ctx, chanBroadcast, payload).Err(); err != nil {
		h.logger.Error("room.broadcast_failed", "room_id", bm.RoomID, "error", err)
		return err
	}
	return nil
}

// removeFromMatchQueue 断线时清理队列
func (h *Hub) removeFromMatchQueue(connID string) {
	h.removeFromMatchQueueContext(h.ctx, connID)
}

func (h *Hub) removeFromMatchQueueContext(ctx context.Context, connID string) {
	entries, err := h.rs.rdb.ZRange(ctx, keyMatchQueue, 0, -1).Result()
	if err != nil {
		h.logger.Error("match.cleanup_failed", "connection_id", connID, "error", err)
		return
	}
	for _, raw := range entries {
		var e queueEntry
		if json.Unmarshal([]byte(raw), &e) == nil && e.ConnID == connID {
			if err := h.rs.rdb.ZRem(ctx, keyMatchQueue, raw).Err(); err != nil {
				h.logger.Error("match.cleanup_failed", "connection_id", connID, "error", err)
			}
		}
	}
}

// aiChoose AI 选择着法
func aiChoose(state *game.GameState) *game.Move {
	return ai.ChooseMove(state, 1)
}
