// Package handler 解析请求、调用业务服务，HTTP 结果由中间件统一输出。
package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"penguin-chess/server/api/request"
	"penguin-chess/server/internal/service"
	apperr "penguin-chess/server/pkg/errors"
)

type Handler struct{ options Options }

func New(options Options) *Handler { return &Handler{options: options} }

func (h *Handler) credentials(r *http.Request, fn func(context.Context, service.Credentials) (*service.Session, error)) (any, error) {
	var in request.Credentials
	if err := request.DecodeJSON(r, &in); err != nil {
		return nil, err
	}
	return fn(r.Context(), service.Credentials{Username: in.Username, Password: in.Password})
}

func (h *Handler) Register(r *http.Request) (any, error) {
	return h.credentials(r, h.options.Accounts.Register)
}
func (h *Handler) Login(r *http.Request) (any, error) {
	return h.credentials(r, h.options.Accounts.Login)
}

func (h *Handler) Profile(r *http.Request) (any, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return nil, apperr.Unauthenticated
	}
	claims, err := h.options.Auth.ParseToken(strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		return nil, apperr.WithCause(apperr.Unauthenticated, err)
	}
	return h.options.Accounts.Profile(r.Context(), claims.UserID)
}

func (h *Handler) Leaderboard(r *http.Request) (any, error) {
	return h.options.Accounts.Leaderboard(r.Context())
}
func (h *Handler) Health(*http.Request) (any, error) { return map[string]string{"status": "ok"}, nil }
func (h *Handler) Ready(r *http.Request) (any, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if h.options.Ready != nil {
		if err := h.options.Ready(ctx); err != nil {
			return nil, apperr.WithCause(apperr.Unavailable, err)
		}
	}
	return map[string]string{"status": "ready"}, nil
}
