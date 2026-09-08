package room

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// 显式启用的本地浏览器夹具：真实 Hub/WebSocket + 隔离 Redis，不连接生产数据库。
func TestBrowserLobbyFixture(t *testing.T) {
	addr := os.Getenv("PENGUIN_BROWSER_FIXTURE_ADDR")
	if addr == "" {
		t.Skip("browser fixture disabled")
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	h := testHub(t)
	peer := NewHub(context.Background(), h.rs.rdb, nil, h.logger, "browser-peer")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := peer.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	done := make(chan struct{})
	var once sync.Once
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, data any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data, "message": "OK"})
	}
	profile := func(name string) map[string]any {
		uid := 1
		if name == "Guest" {
			uid = 2
		}
		return map[string]any{"id": uid, "username": name, "wins": 0, "losses": 0, "draws": 0, "elo": 1000}
	}
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		reply(w, map[string]any{"token": body.Username, "user": profile(body.Username)})
	})
	mux.HandleFunc("/api/profile", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]any{"user": profile(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))})
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		name := r.URL.Query().Get("token")
		uid := int64(0)
		if name == "Host" {
			uid = 1
		}
		if name == "Guest" {
			uid = 2
		}
		hub := h
		if uid == 2 {
			hub = peer
		}
		hub.HandleWS(r.Context(), conn, uid, name)
	})
	mux.HandleFunc("/done", func(w http.ResponseWriter, r *http.Request) { once.Do(func() { close(done) }) })
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go server.Serve(listener)
	t.Log("browser fixture listening", listener.Addr())
	select {
	case <-done:
	case <-time.After(5 * time.Minute):
		t.Fatal("browser fixture timed out")
	}
}
