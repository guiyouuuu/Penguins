package room

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// bindLocal 本地绑定席位
func (h *Hub) bindLocal(c *Client, roomID string, seat int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c.ctx.Err() != nil {
		return
	}
	c.roomID = roomID
	c.seat = seat
	if h.localRooms[roomID] == nil {
		h.localRooms[roomID] = make(map[int]*Client)
	}
	h.localRooms[roomID][seat] = c
	c.logger.Info("room.joined", "room_id", roomID, "seat", seat)
}

// onDisconnect 断线：清理匹配队列 + 通知房间
func (h *Hub) onDisconnect(c *Client) {
	h.workerMu.Lock()
	stopping := h.stopping
	h.workerMu.Unlock()
	if stopping {
		return
	}
	// 已开始的断线清理拥有独立期限，不能被全局取消截断。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if c.room() != "" {
		_ = h.leaveRoomContext(ctx, c)
	} else {
		h.removeFromMatchQueueContext(ctx, c.connID)
	}
}

// leaveRoom 离开房间：广播关房
func (h *Hub) leaveRoom(c *Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = h.leaveRoomContext(ctx, c)
}

func (h *Hub) leaveRoomContext(ctx context.Context, c *Client) error {
	h.mu.Lock()
	roomID, seat := c.roomID, c.seat
	if roomID == "" {
		h.mu.Unlock()
		return nil
	}
	// 本地解绑
	if seats, ok := h.localRooms[roomID]; ok {
		delete(seats, seat)
		if len(seats) == 0 {
			delete(h.localRooms, roomID)
		}
	}
	c.roomID = ""
	c.seat = -1
	h.mu.Unlock()

	// 广播 leave：所有实例关房 + 通知剩余玩家
	payload, _ := json.Marshal(msgSimple{Type: "opponent_left"})
	broadcastErr := h.publishRawContext(ctx, BroadCastMsg{RoomID: roomID, LeaveSeat: seat, BindSeat: -1, Msg: payload})
	err := h.rs.CloseRoomContext(ctx, roomID)
	if err != nil {
		h.logger.Error("room.close_failed", "room_id", roomID, "error", err)
	}
	c.logger.Info("room.left", "room_id", roomID, "seat", seat)
	return errors.Join(broadcastErr, err)
}
