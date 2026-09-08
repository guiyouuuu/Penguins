package room

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"penguin-chess/server/internal/game"
	apperr "penguin-chess/server/pkg/errors"
)

func lobbyClient(h *Hub, uid int64, name string) *Client {
	c := testClient(h)
	c.userID, c.username, c.connID = uid, name, name
	return c
}

func nextMessage(t *testing.T, c *Client, kind string) map[string]any {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case data := <-c.send:
			var msg map[string]any
			if err := json.Unmarshal(data, &msg); err != nil {
				t.Fatal(err)
			}
			if msg["type"] == kind {
				return msg
			}
		case <-timeout:
			t.Fatalf("no %s message", kind)
			return nil
		}
	}
}

func TestFriendRoomRequiresGuestReadyAndHostStart(t *testing.T) {
	h := testHub(t)
	host, guest := lobbyClient(h, 1, "host"), lobbyClient(h, 2, "guest")
	h.createRoom(host)
	code := host.room()
	h.joinRoom(guest, code)
	state, _ := h.rs.GetState(code)
	tile := state.LegalPlacements()[0]
	h.playerMove(host, &game.Move{Kind: "place", From: -1, To: tile})
	msg := nextMessage(t, host, "error")
	if msg["code"] != float64(apperr.BadPhase.Code) {
		t.Fatalf("pre-start move allowed: %v", msg)
	}
	h.handleMsg(host, &inMsg{Type: "start_game"})
	nextMessage(t, host, "error")
	h.handleMsg(guest, &inMsg{Type: "ready", Ready: true})
	h.handleMsg(guest, &inMsg{Type: "start_game"})
	msg = nextMessage(t, guest, "error")
	if msg["code"] != float64(apperr.Forbidden.Code) {
		t.Fatalf("guest can start: %v", msg)
	}
	h.handleMsg(host, &inMsg{Type: "start_game"})
	if h.rs.rdb.HGet(h.ctx, metaKey(code), "started").Val() != "1" {
		t.Fatal("host could not start ready room")
	}
	state, _ = h.rs.GetState(code)
	tile = state.LegalPlacements()[0]
	h.playerMove(host, &game.Move{Kind: "place", From: -1, To: tile})
	state, _ = h.rs.GetState(code)
	if state.Ply != 1 {
		t.Fatal("started room cannot play")
	}
}

func TestGuestLeavesLobbyWithoutClosingHostRoom(t *testing.T) {
	h := testHub(t)
	host, guest := lobbyClient(h, 1, "host"), lobbyClient(h, 2, "guest")
	h.createRoom(host)
	code := host.room()
	h.joinRoom(guest, code)
	h.handleMsg(guest, &inMsg{Type: "ready", Ready: true})
	h.leaveRoom(guest)
	meta, _ := h.rs.GetMeta(code)
	if meta == nil || meta.Full || meta.Seat1UID != 0 || h.rs.rdb.HGet(h.ctx, metaKey(code), "guest_ready").Val() == "1" {
		t.Fatalf("lobby not released: %+v", meta)
	}
	if host.room() != code {
		t.Fatal("host lost room")
	}
	replacement := lobbyClient(h, 3, "replacement")
	h.joinRoom(replacement, code)
	if replacement.room() != code {
		t.Fatal("replacement cannot join")
	}
	h.leaveRoom(host)
	meta, _ = h.rs.GetMeta(code)
	if meta != nil {
		t.Fatal("host leaving did not close room")
	}
}

func TestRoomRejectsSelfJoinAndAIJoin(t *testing.T) {
	h := testHub(t)
	for _, mode := range []string{"pvp", "ai"} {
		code, err := h.rs.CreateRoom(mode, 1, "host")
		if err != nil {
			t.Fatal(err)
		}
		uid := int64(1)
		if mode == "ai" {
			uid = 2
		}
		if err := h.rs.JoinSeat1(code, uid, "invalid"); err == nil {
			t.Fatalf("allowed %s join", mode)
		}
		meta, _ := h.rs.GetMeta(code)
		if meta.Full {
			t.Fatal("rejected join occupied seat")
		}
	}
}

func TestDisconnectWaitsForBusyRoomBeforeReleasingGuest(t *testing.T) {
	h := testHub(t)
	host, guest := lobbyClient(h, 1, "host"), lobbyClient(h, 2, "guest")
	h.createRoom(host)
	code := host.room()
	h.joinRoom(guest, code)
	locked, done := make(chan struct{}), make(chan error, 1)
	go func() {
		done <- h.rs.withLock(code, func() error { close(locked); time.Sleep(350 * time.Millisecond); return nil })
	}()
	<-locked
	h.onDisconnect(guest)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	meta, _ := h.rs.GetMeta(code)
	if meta.Full || guest.room() != "" {
		t.Fatal("disconnect abandoned cleanup while room was busy")
	}
	if h.rs.rdb.Exists(context.Background(), metaKey(code)).Val() != 1 {
		t.Fatal("host room was deleted")
	}
}
