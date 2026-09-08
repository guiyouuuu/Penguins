package room

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"penguin-chess/server/internal/game"
	apperr "penguin-chess/server/pkg/errors"
)

func testHub(t *testing.T) *Hub {
	t.Helper()
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	h := NewHub(context.Background(), client, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)), "test")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := h.Close(ctx); err != nil {
			t.Error(err)
		}
		client.Close()
	})
	return h
}

func testClient(h *Hub) *Client {
	ctx, cancel := context.WithCancel(h.ctx)
	return &Client{hub: h, ctx: ctx, cancel: cancel, logger: h.logger, send: make(chan []byte, 64), connID: "test-connection", seat: -1}
}

func TestConcurrentBroadcastCloseAndMembership(t *testing.T) {
	h := testHub(t)
	c := testClient(h)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				c.writeRaw([]byte(`{"type":"waiting"}`))
				h.bindLocal(c, "TEST", 0)
				c.membership()
			}
			c.close()
		}()
	}
	wg.Wait()
	if c.ctx.Err() == nil {
		t.Fatal("connection was not closed")
	}
}

func TestUnknownAndGameErrorsHaveStableCodes(t *testing.T) {
	h := testHub(t)
	c := testClient(h)
	defer c.close()
	h.handleMsg(c, &inMsg{Type: "unknown"})
	var msg msgError
	if err := json.Unmarshal(<-c.send, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Code != apperr.InvalidArgument.Code || msg.Msg != msg.Message {
		t.Fatalf("unexpected error: %+v", msg)
	}
	sendError(c, game.ErrNotYourTurn)
	if err := json.Unmarshal(<-c.send, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Code != apperr.NotYourTurn.Code {
		t.Fatal("game error not mapped")
	}
}

func TestCommandPanicIsRecoveredAndSubsequentCommandsWork(t *testing.T) {
	h := testHub(t)
	c := testClient(h)
	defer c.close()
	h.rs = nil
	h.handleMsg(c, &inMsg{Type: "play_ai"})
	var msg msgError
	if err := json.Unmarshal(<-c.send, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Code != apperr.Internal.Code {
		t.Fatal("panic not converted")
	}
	h.handleMsg(c, &inMsg{Type: "unknown"})
	if err := json.Unmarshal(<-c.send, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Code != apperr.InvalidArgument.Code {
		t.Fatal("command handling did not recover")
	}
}

func TestRoomJoinUsesSecondsForTTL(t *testing.T) {
	h := testHub(t)
	code, err := h.rs.CreateRoom("pvp", 1, "one")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.rs.JoinSeat1(code, 2, "two"); err != nil {
		t.Fatal(err)
	}
	ttl, err := h.rs.rdb.TTL(h.ctx, metaKey(code)).Result()
	if err != nil {
		t.Fatal(err)
	}
	if ttl > roomTTL || ttl < roomTTL-time.Second {
		t.Fatalf("unexpected TTL: %s", ttl)
	}
}

func TestShutdownCleansWaitingPlayersAfterWorkersStop(t *testing.T) {
	h := testHub(t)
	c := testClient(h)
	c.userID, c.username = 10, "waiting"
	h.conns[c.connID] = c
	entry := queueEntry{ConnID: c.connID, InstanceID: h.instanceID, UID: c.userID, Name: c.username}
	h.run("test.late_enqueue", func() {
		<-h.ctx.Done()
		data, _ := json.Marshal(entry)
		if err := h.rs.rdb.ZAdd(context.Background(), keyMatchQueue, redis.Z{Member: string(data)}).Err(); err != nil {
			t.Error(err)
		}
		h.onDisconnect(c)
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if count := h.rs.rdb.ZCard(context.Background(), keyMatchQueue).Val(); count != 0 {
		t.Fatalf("shutdown left %d matching players", count)
	}
}

type blockingRedisHook struct{}

func (blockingRedisHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) { return next(ctx, network, addr) }
}
func (blockingRedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "publish" || cmd.Name() == "zrem" {
			<-ctx.Done()
			return ctx.Err()
		}
		return next(ctx, cmd)
	}
}
func (blockingRedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		<-ctx.Done()
		return ctx.Err()
	}
}

func TestShutdownDeadlineIncludesRedisCleanup(t *testing.T) {
	h := testHub(t)
	c := testClient(h)
	h.conns[c.connID] = c
	h.bindLocal(c, "TEST", 0)
	h.rs.rdb.AddHook(blockingRedisHook{})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := h.Close(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("shutdown exceeded deadline: %s", elapsed)
	}
	// The shared helper calls Close again during cleanup.
	h.mu.Lock()
	delete(h.conns, c.connID)
	h.mu.Unlock()
}

func TestMatchLockReleaseDoesNotDeleteNewOwner(t *testing.T) {
	h := testHub(t)
	if err := h.rs.rdb.Set(h.ctx, keyMatchLock, "new-owner", matchLockTTL).Err(); err != nil {
		t.Fatal(err)
	}
	h.releaseMatchLock("expired-owner")
	if value := h.rs.rdb.Get(h.ctx, keyMatchLock).Val(); value != "new-owner" {
		t.Fatalf("deleted another matcher's lock: %q", value)
	}
}

func TestInFlightLeaveSurvivesHubCancellation(t *testing.T) {
	h := testHub(t)
	c := testClient(h)
	roomID, err := h.rs.CreateRoom("pvp", 1, "one")
	if err != nil {
		t.Fatal(err)
	}
	h.bindLocal(c, roomID, 0)
	h.cancel()
	h.leaveRoom(c)
	if exists := h.rs.rdb.Exists(context.Background(), metaKey(roomID)).Val(); exists != 0 {
		t.Fatal("canceled hub left room metadata behind")
	}
}
