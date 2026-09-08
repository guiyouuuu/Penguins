package room

import (
	"runtime/debug"
	"strings"
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
	case "ready":
		h.updateLobby(c, false, m.Ready)
	case "start_game":
		h.updateLobby(c, true, false)
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
	c.writeJSON(lobbyMessage(code, &RoomMeta{Seat0Name: c.username}))
}

func (h *Hub) joinRoom(c *Client, code string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if c.userID <= 0 {
		sendError(c, apperr.Unauthenticated)
		return
	}
	if c.room() != "" {
		sendError(c, apperr.AlreadyInRoom)
		return
	}
	if len(code) != 4 || strings.Trim(code, roomCodeChars) != "" {
		sendError(c, apperr.InvalidArgument)
		return
	}
	err := h.rs.withLock(code, func() error {
		meta, err := h.rs.GetMeta(code)
		if err != nil {
			return err
		}
		if meta == nil || meta.Mode != "pvp" {
			return apperr.RoomGone
		}
		if err := h.rs.JoinSeat1(code, c.userID, c.username); err != nil {
			return err
		}
		meta.Seat1UID, meta.Seat1Name, meta.Full, meta.GuestReady = c.userID, c.username, true, false
		h.bindLocal(c, code, 1)
		c.writeJSON(msgRoom{Type: "room", Room: code, You: 1})
		return h.rs.Publish(code, lobbyMessage(code, meta))
	})
	if err != nil {
		sendError(c, err)
	}
}

// 好友房在双方到齐且客人准备后，由房主显式开局。
func (h *Hub) updateLobby(c *Client, start, ready bool) {
	code, seat := c.membership()
	if code == "" {
		sendError(c, apperr.NotInRoom)
		return
	}
	err := h.rs.withLock(code, func() error {
		meta, err := h.rs.GetMeta(code)
		if err != nil {
			return err
		}
		if meta == nil {
			return apperr.RoomGone
		}
		if meta.Mode != "pvp" || meta.Started {
			return apperr.BadPhase
		}
		if start {
			if seat != 0 || c.userID != meta.Seat0UID {
				return apperr.Forbidden
			}
			if !meta.Full || !meta.GuestReady {
				return apperr.RoomNotReady
			}
			state, err := h.rs.ResetForRematch(code, meta)
			if err != nil {
				return err
			}
			meta.Started = true
			if err := h.rs.saveMeta(code, meta); err != nil {
				return err
			}
			return h.rs.Publish(code, msgStart{Type: "start", State: state, Names: [2]string{meta.Seat0Name, meta.Seat1Name}})
		}
		if seat != 1 || c.userID != meta.Seat1UID {
			return apperr.Forbidden
		}
		meta.GuestReady = ready
		if err := h.rs.saveMeta(code, meta); err != nil {
			return err
		}
		return h.rs.Publish(code, lobbyMessage(code, meta))
	})
	if err != nil {
		sendError(c, err)
	}
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
