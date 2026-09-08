package middleware

import (
	"bufio"
	"context"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	"penguin-chess/server/api/response"

	"github.com/google/uuid"

	apperr "penguin-chess/server/pkg/errors"
	"penguin-chess/server/pkg/requestctx"
)

// responseWriter 保留 Hijacker，确保访问日志中间件不破坏 WebSocket 升级。
type responseWriter struct {
	http.ResponseWriter
	status, bytes int
	hijacked      bool
}

func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *responseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *responseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}
func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil {
		w.hijacked = true
		w.status = http.StatusSwitchingProtocols
	}
	return conn, rw, err
}

func HTTP(logger *slog.Logger, timeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := uuid.NewString()
		ctx := requestctx.WithID(r.Context(), id)
		// 升级后的连接有独立读写期限，不能继承 HTTP 请求超时。
		if r.URL.Path != "/ws" {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		r = r.WithContext(ctx)
		rw := &responseWriter{ResponseWriter: w}
		rw.Header().Set("X-Request-ID", id)
		rw.Header().Set("X-Content-Type-Options", "nosniff")
		r.Body = http.MaxBytesReader(rw, r.Body, 16<<10)
		defer func() {
			if value := recover(); value != nil {
				logger.ErrorContext(ctx, "request.panic", "request_id", id, "path", r.URL.Path, "panic", value, "stack", string(debug.Stack()))
				if rw.status == 0 && !rw.hijacked {
					response.Fail(rw, r, apperr.Internal)
				}
			}
			status := rw.status
			if status == 0 {
				status = http.StatusOK
			}
			logger.InfoContext(ctx, "http.access", "request_id", id, "method", r.Method, "path", r.URL.Path, "status", status, "bytes", rw.bytes, "duration_ms", time.Since(start).Milliseconds(), "remote_addr", r.RemoteAddr)
		}()
		next.ServeHTTP(rw, r)
	})
}
