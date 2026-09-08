package middleware

import (
	"errors"
	"log/slog"
	"net/http"

	"penguin-chess/server/api/response"
	apperr "penguin-chess/server/pkg/errors"
	"penguin-chess/server/pkg/requestctx"
)

type Endpoint func(*http.Request) (any, error)

// Adapt 集中处理业务返回和错误日志，handler 只返回结果或 error。
func Adapt(logger *slog.Logger, fn Endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := fn(r)
		if err != nil {
			public := apperr.Resolve(err)
			level := slog.LevelWarn
			if public.Status >= 500 {
				level = slog.LevelError
			}
			logger.Log(r.Context(), level, "request.failed", "request_id", requestctx.ID(r.Context()), "path", r.URL.Path, "code", public.Code, "error", err, "cause", errors.Unwrap(public))
			response.Fail(w, r, err)
			return
		}
		response.Success(w, r, value)
	}
}
