package room

import (
	"context"
	"encoding/json"
	"errors"
	apperr "penguin-chess/server/pkg/errors"
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
		if err := h.leaveRoomContext(ctx, c); err != nil {
			c.logger.Error("room.disconnect_cleanup_failed", "error", err)
		}
	} else {
		h.removeFromMatchQueueContext(ctx, c.connID)
	}
}

// leaveRoom 离开房间：广播关房
func (h *Hub) leaveRoom(c *Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.leaveRoomContext(ctx, c); err != nil {
		c.logger.Error("room.leave_failed", "error", err)
		sendError(c, err)
	}
}

func (h *Hub) leaveRoomContext(ctx context.Context, c *Client) error {
	roomID, _ := c.membership()
	if roomID == "" {
		return nil
	}
	// 离房不能因一次普通操作的短暂锁竞争而放弃清理。
	for {
		err := h.rs.withLockContext(ctx, roomID, func() error { return h.leaveLocked(ctx, c) })
		if !errors.Is(err, apperr.RateLimited) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func (h *Hub) leaveLocked(ctx context.Context, c *Client) error {
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
	// 使用清理期限读取元数据，关闭服务时也能完成离房处理。
	store := NewRoomStore(ctx, h.rs.rdb)
	meta, err := store.GetMeta(roomID)
	if err != nil {
		return err
	}
	if meta != nil && meta.Mode == "pvp" && !meta.Started && seat == 1 {
		meta.Seat1UID, meta.Seat1Name, meta.Full, meta.GuestReady = 0, "", false, false
		if err := store.saveMeta(roomID, meta); err != nil {
			return err
		}
		payload, _ := json.Marshal(lobbyMessage(roomID, meta))
		return h.publishRawContext(ctx, BroadCastMsg{RoomID: roomID, LeaveSeat: seat, BindSeat: -1, KeepRoom: true, Msg: payload})
	}

	// 广播 leave：所有实例关房 + 通知剩余玩家
	payload, _ := json.Marshal(msgSimple{Type: "opponent_left"})
	broadcastErr := h.publishRawContext(ctx, BroadCastMsg{RoomID: roomID, LeaveSeat: seat, BindSeat: -1, Msg: payload})
	err = h.rs.CloseRoomContext(ctx, roomID)
	if err != nil {
		h.logger.Error("room.close_failed", "room_id", roomID, "error", err)
	}
	c.logger.Info("room.left", "room_id", roomID, "seat", seat)
	return errors.Join(broadcastErr, err)
}
