package room

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	apperr "penguin-chess/server/pkg/errors"
	"penguin-chess/server/pkg/requestctx"
)

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	connID   string
	userID   int64
	username string
	// 席位只在 hub.mu 下读写，后台匹配和订阅任务也遵守这一约束。
	roomID    string
	seat      int
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
	logger    *slog.Logger
}

func (c *Client) membership() (string, int) {
	c.hub.mu.RLock()
	defer c.hub.mu.RUnlock()
	return c.roomID, c.seat
}
func (c *Client) room() string   { room, _ := c.membership(); return room }
func (c *Client) seatIndex() int { _, seat := c.membership(); return seat }
func (c *Client) close() {
	c.closeOnce.Do(func() {
		c.cancel()
		if c.conn != nil {
			_ = c.conn.Close()
		}
	})
}

func (c *Client) readLoop() {
	defer func() {
		c.close()
		c.hub.onDisconnect(c)
		c.hub.mu.Lock()
		delete(c.hub.conns, c.connID)
		c.hub.mu.Unlock()
		c.logger.Info("websocket.disconnected")
	}()
	c.conn.SetReadLimit(16 << 10)
	_ = c.conn.SetReadDeadline(time.Now().Add(65 * time.Second))
	c.conn.SetPongHandler(func(string) error { return c.conn.SetReadDeadline(time.Now().Add(65 * time.Second)) })
	for {
		kind, data, err := c.conn.ReadMessage()
		if err != nil {
			if c.ctx.Err() == nil && websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				c.logger.Warn("websocket.read_failed", "error", err)
			}
			return
		}
		var msg inMsg
		if kind != websocket.TextMessage {
			sendError(c, apperr.InvalidArgument)
			continue
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			sendError(c, apperr.WithCause(apperr.InvalidArgument, err))
			continue
		}
		c.hub.handleMsg(c, &msg)
	}
}

func (c *Client) writeLoop() {
	defer c.close()
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case data := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				c.logger.Warn("websocket.write_failed", "error", err)
				return
			}
		case <-ticker.C:
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
				return
			}
		}
	}
}

func (c *Client) writeJSON(value any) {
	data, err := json.Marshal(value)
	if err != nil {
		c.logger.Error("websocket.encode_failed", "error", err)
		return
	}
	c.writeRaw(data)
}

// send 不关闭，使用 context 通知写循环退出，避免广播与断线并发时 send-on-closed panic。
func (c *Client) writeRaw(data json.RawMessage) {
	if c.ctx.Err() != nil {
		return
	}
	select {
	case <-c.ctx.Done():
		return
	case c.send <- []byte(data):
	default:
		c.logger.Warn("websocket.slow_consumer", "queue_capacity", cap(c.send))
		c.close()
	}
}

func sendError(c *Client, err error) {
	public := publicError(err)
	level := slog.LevelWarn
	if public.Status >= 500 {
		level = slog.LevelError
	}
	c.logger.Log(c.ctx, level, "websocket.command_failed", "room_id", c.room(), "code", public.Code, "error", err, "cause", public.Unwrap())
	c.writeJSON(msgError{Type: "error", Code: public.Code, Message: public.Message, Msg: public.Message, RequestID: requestctx.ID(c.ctx)})
}
