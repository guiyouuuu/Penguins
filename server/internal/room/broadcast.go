package room

import (
	"encoding/json"

	"github.com/redis/go-redis/v9"

	logging "penguin-chess/server/pkg/logger"
)

// publishRoom 发布房间广播
func (h *Hub) publishRoom(roomID string, msg any) {
	if err := h.rs.Publish(roomID, msg); err != nil {
		h.logger.Error("room.broadcast_failed", "room_id", roomID, "error", err)
	}
}

// ---------- Pub/Sub 订阅 ----------

// subscribeLoop 订阅广播频道，转发到本地连接
func (h *Hub) subscribeLoop(rdb *redis.Client) {
	sub := rdb.Subscribe(h.ctx, chanBroadcast)
	defer sub.Close()
	for {
		var msg *redis.Message
		select {
		case <-h.ctx.Done():
			return
		case value, ok := <-sub.Channel():
			if !ok {
				return
			}
			msg = value
		}
		var bm BroadCastMsg
		if err := json.Unmarshal([]byte(msg.Payload), &bm); err != nil {
			h.logger.Error("room.broadcast_decode_failed", "error", err)
			continue
		}
		h.dispatch(bm)
	}
}

// dispatch 处理一条广播消息
func (h *Hub) dispatch(bm BroadCastMsg) {
	defer logging.Recover(h.logger, "broadcast.dispatch")
	// 1. 离房：关房 + 通知剩余玩家
	if bm.LeaveSeat >= 0 && !bm.KeepRoom {
		h.mu.Lock()
		seats := h.localRooms[bm.RoomID]
		var rest []*Client
		for s, c := range seats {
			if s != bm.LeaveSeat {
				rest = append(rest, c)
			}
		}
		delete(h.localRooms, bm.RoomID)
		for _, c := range rest {
			c.roomID = ""
			c.seat = -1
		}
		h.mu.Unlock()
		for _, c := range rest {
			c.writeJSON(msgSimple{Type: "opponent_left"})
		}
		return
	}

	// 2. 定向：绑定席位 + 发消息
	if bm.TargetConn != "" {
		h.mu.RLock()
		c := h.conns[bm.TargetConn]
		h.mu.RUnlock()
		if c == nil {
			return
		}
		if bm.BindSeat >= 0 {
			h.bindLocal(c, bm.RoomID, bm.BindSeat)
		}
		if len(bm.Msg) > 0 {
			c.writeRaw(bm.Msg)
		}
		return
	}

	// 3. 房间广播
	h.mu.RLock()
	seats := h.localRooms[bm.RoomID]
	targets := make([]*Client, 0, len(seats))
	for _, c := range seats {
		targets = append(targets, c)
	}
	h.mu.RUnlock()
	for _, c := range targets {
		c.writeRaw(bm.Msg)
	}
}
