// Package room 管理实时连接、房间和对局，Redis 协调跨实例状态。
package room

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"penguin-chess/server/internal/repository/mysql"
	logging "penguin-chess/server/pkg/logger"
	"penguin-chess/server/pkg/requestctx"
)

type Hub struct {
	rs         *RoomStore
	db         *mysql.DB
	logger     *slog.Logger
	instanceID string
	ctx        context.Context
	cancel     context.CancelFunc
	workers    sync.WaitGroup
	workerMu   sync.Mutex
	stopping   bool
	mu         sync.RWMutex
	conns      map[string]*Client
	localRooms map[string]map[int]*Client
}

func waitFor(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func NewHub(parent context.Context, rdb *redis.Client, db *mysql.DB, logger *slog.Logger, instanceID string) *Hub {
	ctx, cancel := context.WithCancel(parent)
	h := &Hub{ctx: ctx, cancel: cancel, rs: NewRoomStore(ctx, rdb), db: db, logger: logger.With("component", "room", "instance_id", instanceID), instanceID: instanceID, conns: make(map[string]*Client), localRooms: make(map[string]map[int]*Client)}
	h.run("broadcast.subscribe", func() { h.subscribeLoop(rdb) })
	return h
}

// run 统一回收后台任务；关闭期间不再增加 WaitGroup 计数。
func (h *Hub) run(operation string, fn func()) bool {
	h.workerMu.Lock()
	defer h.workerMu.Unlock()
	if h.stopping {
		return false
	}
	h.workers.Add(1)
	go func() { defer h.workers.Done(); defer logging.Recover(h.logger, operation); fn() }()
	return true
}

func (h *Hub) HandleWS(request context.Context, conn *websocket.Conn, userID int64, username string) {
	ctx, cancel := context.WithCancel(requestctx.WithID(h.ctx, requestctx.ID(request)))
	c := &Client{hub: h, conn: conn, send: make(chan []byte, 64), connID: uuid.NewString(), userID: userID, username: username, seat: -1, ctx: ctx, cancel: cancel}
	c.logger = h.logger.With("connection_id", c.connID, "user_id", userID, "request_id", requestctx.ID(request))
	h.workerMu.Lock()
	if h.stopping {
		h.workerMu.Unlock()
		c.close()
		return
	}
	h.mu.Lock()
	h.conns[c.connID] = c
	h.mu.Unlock()
	h.workerMu.Unlock()
	c.logger.Info("websocket.connected")
	if !h.run("websocket.write", c.writeLoop) || !h.run("websocket.read", c.readLoop) {
		c.close()
		h.mu.Lock()
		delete(h.conns, c.connID)
		h.mu.Unlock()
	}
}

func (h *Hub) Close(ctx context.Context) error {
	h.workerMu.Lock()
	h.stopping = true
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.conns))
	for _, c := range h.conns {
		clients = append(clients, c)
	}
	h.mu.RUnlock()
	h.workerMu.Unlock()
	h.cancel()
	for _, c := range clients {
		c.close()
	}
	done := make(chan struct{})
	go func() { h.workers.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	// 后台任务完全退出后再清队列，避免延迟匹配在清理之后重新入队。
	var result error
	for _, c := range clients {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		result = errors.Join(result, h.leaveRoomContext(ctx, c))
		entry, _ := json.Marshal(queueEntry{ConnID: c.connID, InstanceID: h.instanceID, UID: c.userID, Name: c.username})
		if err := h.rs.rdb.ZRem(ctx, keyMatchQueue, string(entry)).Err(); err != nil {
			h.logger.Error("match.cleanup_failed", "connection_id", c.connID, "error", err)
			result = errors.Join(result, err)
		}
	}
	return result
}
