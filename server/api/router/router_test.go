package router

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"penguin-chess/server/api/handler"
	"penguin-chess/server/api/response"

	"github.com/gorilla/websocket"

	"penguin-chess/server/internal/service"
	"penguin-chess/server/pkg/auth"
)

type fakeAccounts struct {
	fail  bool
	crash bool
}

func (a fakeAccounts) Register(context.Context, service.Credentials) (*service.Session, error) {
	return &service.Session{}, nil
}
func (a fakeAccounts) Login(context.Context, service.Credentials) (*service.Session, error) {
	if a.crash {
		panic("private panic detail")
	}
	if a.fail {
		return nil, errors.New("private database address")
	}
	return &service.Session{}, nil
}
func (a fakeAccounts) Profile(context.Context, int64) (*service.Profile, error) {
	return &service.Profile{}, nil
}
func (a fakeAccounts) Leaderboard(context.Context) (*service.Leaderboard, error) {
	return &service.Leaderboard{}, nil
}

type fakeHub struct{}

func (fakeHub) HandleWS(_ context.Context, c *websocket.Conn, _ int64, _ string) {
	defer c.Close()
	_ = c.WriteJSON(map[string]string{"type": "waiting"})
}

func testRouter(a fakeAccounts) http.Handler {
	return New(handler.Options{Accounts: a, Auth: auth.New("test-secret"), Hub: fakeHub{}, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), RequestTimeout: time.Second})
}

func TestEnvelopeAndCentralErrors(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body string
		status, code             int
		accounts                 fakeAccounts
	}{
		{name: "success", method: "POST", path: "/api/login", body: `{"username":"tester","password":"password"}`, status: 200},
		{name: "bad json", method: "POST", path: "/api/login", body: `{`, status: 400, code: 10001},
		{name: "unknown field", method: "POST", path: "/api/login", body: `{"extra":true}`, status: 400, code: 10001},
		{name: "trailing json", method: "POST", path: "/api/login", body: `{} {}`, status: 400, code: 10001},
		{name: "oversized", method: "POST", path: "/api/login", body: `{"username":"` + strings.Repeat("x", 20000) + `"}`, status: 413, code: 10004},
		{name: "unauthorized", method: "GET", path: "/api/profile", status: 401, code: 20001},
		{name: "not found", method: "GET", path: "/api/no-such-path", status: 404, code: 10002},
		{name: "wrong method", method: "GET", path: "/api/login", status: 405, code: 10003},
		{name: "internal", method: "POST", path: "/api/login", body: `{}`, status: 500, code: 50000, accounts: fakeAccounts{fail: true}},
		{name: "panic", method: "POST", path: "/api/login", body: `{}`, status: 500, code: 50000, accounts: fakeAccounts{crash: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := testRouter(tc.accounts)
			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			handler.ServeHTTP(w, r)
			var got response.Response
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || got.Code != tc.code || got.RequestID == "" || got.RequestID != w.Header().Get("X-Request-ID") {
				t.Fatalf("unexpected envelope: %d %+v", w.Code, got)
			}
			if strings.Contains(w.Body.String(), "private") {
				t.Fatal("internal detail leaked")
			}
			if tc.code != 0 && got.Data != nil {
				t.Fatal("error data must be null")
			}
		})
	}
}

func TestMiddlewarePreservesWebSocketUpgrade(t *testing.T) {
	server := httptest.NewServer(testRouter(fakeAccounts{}))
	defer server.Close()
	conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if response.Header.Get("X-Request-ID") == "" {
		t.Fatal("missing request id")
	}
	var msg map[string]string
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatal(err)
	}
	if msg["type"] != "waiting" {
		t.Fatal("lost websocket frame")
	}
}

func TestInvalidWebSocketTokenIsNotDowngradedToGuest(t *testing.T) {
	w := httptest.NewRecorder()
	testRouter(fakeAccounts{}).ServeHTTP(w, httptest.NewRequest("GET", "/ws?token=invalid", nil))
	if w.Code != 401 {
		t.Fatalf("got %d", w.Code)
	}
}

func TestWebSocketOriginsBehindTunnel(t *testing.T) {
	server := httptest.NewServer(New(handler.Options{
		Auth: auth.New("test-secret"), Hub: fakeHub{},
		Logger:           slog.New(slog.NewJSONHandler(io.Discard, nil)),
		WSAllowedOrigins: []string{"https://penguins.iepose.cn"},
	}))
	defer server.Close()
	for _, tc := range []struct {
		name    string
		origins []string
		allowed bool
	}{
		{"tunnel rewrites host", []string{"https://penguins.iepose.cn"}, true},
		{"same origin", []string{server.URL}, true},
		{"non browser client", nil, true},
		{"unknown origin", []string{"https://other.example"}, false},
		{"suffix attack", []string{"https://penguins.iepose.cn.other.example"}, false},
		{"wrong scheme", []string{"http://penguins.iepose.cn"}, false},
		{"wrong port", []string{"https://penguins.iepose.cn:8443"}, false},
		{"opaque origin", []string{"null"}, false},
		{"userinfo", []string{"https://user@penguins.iepose.cn"}, false},
		{"path", []string{"https://penguins.iepose.cn/path"}, false},
		{"duplicate origins", []string{"https://penguins.iepose.cn", "https://other.example"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{"X-Forwarded-Host": []string{"other.example"}}
			if tc.origins != nil {
				headers["Origin"] = tc.origins
			}
			conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", headers)
			if tc.allowed {
				if err != nil {
					t.Fatalf("expected accepted handshake: %v", err)
				}
				defer conn.Close()
				var msg map[string]string
				if err := conn.ReadJSON(&msg); err != nil {
					t.Fatal(err)
				}
				if msg["type"] != "waiting" {
					t.Fatal("missing websocket frame")
				}
			} else {
				if conn != nil {
					conn.Close()
				}
				if response != nil {
					defer response.Body.Close()
				}
				if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
					t.Fatalf("expected forbidden handshake, got %v, %v", response, err)
				}
			}
		})
	}
}
