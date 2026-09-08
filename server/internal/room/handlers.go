package room

import (
	"runtime/debug"
	"time"

	"penguin-chess/server/internal/game"
	apperr "penguin-chess/server/pkg/errors"
)

func (h *Hub) handleMsg(c *Client, m *inMsg) {
	start := time.Now()
	defer func() {
		if value := recover(); value != nil {
			c.logger.Error("websocket.command_panic", "panic", value, "stack", string(debug.Stack()))
			sendError(c, apperr.Internal)
		}
		c.logger.Debug("websocket.command", "type", m.Type, "room_id", c.room(), "duration_ms", time.Since(start).Milliseconds())
	}()
	switch m.Type {
	case "create_room":
		h.createRoom(c)
	case "join_room":
		h.joinRoom(c, m.Room)
	case "quick_match":
		h.quickMatch(c)
	case "play_ai":
		h.playAI(c)
	case "place":
		h.playerMove(c, &game.Move{Kind: "place", Player: c.seatIndex(), From: -1, To: m.Tile})
	case "move":
		h.playerMove(c, &game.Move{Kind: "move", Player: c.seatIndex(), From: m.From, To: m.To})
	case "resign":
		h.resign(c)
	case "rematch":
		h.rematch(c)
	case "leave":
		h.leaveRoom(c)
	default:
		sendError(c, apperr.InvalidArgument)
	}
}

func (h *Hub) createRoom(c *Client) {
	if c.userID <= 0 {
		sendError(c, apperr.Unauthenticated)
		return
	}
	if c.room() != "" {
		sendError(c, apperr.AlreadyInRoom)
		return
	}
	code, err := h.rs.CreateRoom("pvp", c.userID, c.username)
	if err != nil {
		sendError(c, err)
		return
	}
	h.bindLocal(c, code, 0)
	c.writeJSON(msgRoom{Type: "room", Room: code, You: 0})
	c.writeJSON(msgWaiting{Type: "waiting"})
}

func (h *Hub) joinRoom(c *Client, code string) {
	if c.userID <= 0 {
		sendError(c, apperr.Unauthenticated)
		return
	}
	if c.room() != "" {
		sendError(c, apperr.AlreadyInRoom)
		return
	}
	meta, err := h.rs.GetMeta(code)
	if err != nil {
		sendError(c, err)
		return
	}
	if meta == nil {
		sendError(c, apperr.RoomGone)
		return
	}
	if err := h.rs.JoinSeat1(code, c.userID, c.username); err != nil {
		sendError(c, err)
		return
	}
	h.bindLocal(c, code, 1)
	c.writeJSON(msgRoom{Type: "room", Room: code, You: 1})

	// 广播开局
	meta.Seat1UID = c.userID
	meta.Seat1Name = c.username
	state, err := h.rs.GetState(code)
	if err != nil {
		sendError(c, err)
		return
	}
	if state == nil {
		sendError(c, apperr.RoomGone)
		return
	}
	state.Players[1].Name = c.username
	state.Players[1].UserID = c.userID
	if err := h.rs.saveState(code, state); err != nil {
		sendError(c, err)
		return
	}
	names := [2]string{state.Players[0].Name, state.Players[1].Name}
	h.publishRoom(code, msgStart{Type: "start", State: state, Names: names})
}

func (h *Hub) playAI(c *Client) {
	if c.room() != "" {
		sendError(c, apperr.AlreadyInRoom)
		return
	}
	code, err := h.rs.CreateRoom("ai", c.userID, c.username)
	if err != nil {
		sendError(c, err)
		return
	}
	h.bindLocal(c, code, 0)
	c.writeJSON(msgRoom{Type: "room", Room: code, You: 0})
	state, err := h.rs.GetState(code)
	if err != nil {
		sendError(c, err)
		h.leaveRoom(c)
		return
	}
	if state == nil {
		sendError(c, apperr.RoomGone)
		h.leaveRoom(c)
		return
	}
	names := [2]string{state.Players[0].Name, state.Players[1].Name}
	c.writeJSON(msgStart{Type: "start", State: state, Names: names})
}
