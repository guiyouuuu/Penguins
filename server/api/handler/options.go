package handler

import (
	"context"
	"io/fs"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"

	"penguin-chess/server/internal/service"
	"penguin-chess/server/pkg/auth"
)

type Accounts interface {
	Register(context.Context, service.Credentials) (*service.Session, error)
	Login(context.Context, service.Credentials) (*service.Session, error)
	Profile(context.Context, int64) (*service.Profile, error)
	Leaderboard(context.Context) (*service.Leaderboard, error)
}
type Connections interface {
	HandleWS(context.Context, *websocket.Conn, int64, string)
}
type Options struct {
	Accounts         Accounts
	Auth             *auth.Service
	Hub              Connections
	Static           fs.FS
	Logger           *slog.Logger
	Ready            func(context.Context) error
	RequestTimeout   time.Duration
	WSAllowedOrigins []string
}
