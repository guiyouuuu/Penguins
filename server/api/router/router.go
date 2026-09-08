// Package router 只负责 URL 注册及全局中间件绑定。
package router

import (
	"log/slog"
	"net/http"
	"time"

	"penguin-chess/server/api/handler"
	"penguin-chess/server/api/response"
	"penguin-chess/server/middleware"
	apperr "penguin-chess/server/pkg/errors"
)

func New(o handler.Options) http.Handler {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.RequestTimeout <= 0 {
		o.RequestTimeout = 10 * time.Second
	}
	h := handler.New(o)
	mux := http.NewServeMux()
	register := func(method, path string, fn middleware.Endpoint) {
		mux.HandleFunc(method+" "+path, middleware.Adapt(o.Logger, fn))
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Allow", method)
			response.Fail(w, r, apperr.MethodNotAllowed)
		})
	}
	register("POST", "/api/register", h.Register)
	register("POST", "/api/login", h.Login)
	register("GET", "/api/profile", h.Profile)
	register("GET", "/api/leaderboard", h.Leaderboard)
	register("GET", "/healthz", h.Health)
	register("GET", "/readyz", h.Ready)
	notFound := func(w http.ResponseWriter, r *http.Request) { response.Fail(w, r, apperr.NotFound) }
	mux.HandleFunc("/api/", notFound)
	mux.HandleFunc("/api", notFound)
	mux.HandleFunc("GET /ws", h.WebSocket())
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", "GET")
		response.Fail(w, r, apperr.MethodNotAllowed)
	})
	if o.Static != nil {
		mux.Handle("/", handler.Static(o.Static))
	} else {
		mux.HandleFunc("/", notFound)
	}
	return middleware.HTTP(o.Logger, o.RequestTimeout, mux)
}
