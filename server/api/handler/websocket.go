package handler

import (
	"net/http"

	"github.com/gorilla/websocket"

	"penguin-chess/server/api/response"
	apperr "penguin-chess/server/pkg/errors"
	"penguin-chess/server/pkg/requestctx"
)

func (h *Handler) WebSocket() http.HandlerFunc {
	o := h.options
	upgrader := websocket.Upgrader{ReadBufferSize: 2048, WriteBufferSize: 2048, Error: func(w http.ResponseWriter, r *http.Request, status int, _ error) {
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
