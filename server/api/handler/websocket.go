package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"

	"penguin-chess/server/api/response"
	apperr "penguin-chess/server/pkg/errors"
	"penguin-chess/server/pkg/requestctx"
)

func (h *Handler) WebSocket() http.HandlerFunc {
	o := h.options
	upgrader := websocket.Upgrader{ReadBufferSize: 2048, WriteBufferSize: 2048, CheckOrigin: func(r *http.Request) bool {
		origins := r.Header.Values("Origin")
		if len(origins) == 0 {
			return true
		}
		if len(origins) != 1 {
			return false
		}
		origin, err := url.Parse(origins[0])
		if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" {
			return false
		}
		if strings.EqualFold(origin.Host, r.Host) {
			return true
		}
		// 穿透可能改写 Host；仅信任显式来源白名单，不信任转发头。
		for _, allowed := range o.WSAllowedOrigins {
			if strings.EqualFold(origin.Scheme+"://"+origin.Host, allowed) {
				return true
			}
		}
		return false
	}, Error: func(w http.ResponseWriter, r *http.Request, status int, _ error) {
		if status == http.StatusForbidden {
			response.Fail(w, r, apperr.Forbidden)
		} else {
			response.Fail(w, r, apperr.InvalidArgument)
		}
	}}
	return func(w http.ResponseWriter, r *http.Request) {
		uid := int64(0)
		username := "游客"
		if token := r.URL.Query().Get("token"); token != "" {
			claims, err := o.Auth.ParseToken(token)
			if err != nil {
				response.Fail(w, r, apperr.Unauthenticated)
				return
			}
			uid = claims.UserID
			username = claims.Username
		}
		conn, err := upgrader.Upgrade(w, r, http.Header{"X-Request-ID": []string{requestctx.ID(r.Context())}})
		if err != nil {
			o.Logger.WarnContext(r.Context(), "websocket.upgrade_failed", "request_id", requestctx.ID(r.Context()), "error", err)
			return
		}
		o.Hub.HandleWS(r.Context(), conn, uid, username)
	}
}
