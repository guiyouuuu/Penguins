// Package service 实现业务流程，避免 HTTP handler 直接操作数据库。
package service

import (
	"context"
	"errors"
	"fmt"

	"penguin-chess/server/model/entity"

	"penguin-chess/server/internal/repository/mysql"
	"penguin-chess/server/pkg/auth"
	apperr "penguin-chess/server/pkg/errors"
)

type UserRepository interface {
	CreateUser(context.Context, string, string) (int64, error)
	GetUserByName(context.Context, string) (*entity.User, string, error)
	GetUserByID(context.Context, int64) (*entity.User, error)
	RecentGames(context.Context, int64, int) ([]entity.GameRecord, error)
	TopPlayers(context.Context, int) ([]entity.User, error)
}

type Account struct {
	users UserRepository
	auth  *auth.Service
}

func NewAccount(users UserRepository, authSvc *auth.Service) *Account {
	return &Account{users: users, auth: authSvc}
}

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type Session struct {
	Token string      `json:"token"`
	User  entity.User `json:"user"`
}
type Profile struct {
	User   entity.User         `json:"user"`
	Recent []entity.GameRecord `json:"recent"`
}
type Leaderboard struct {
	Players []entity.User `json:"players"`
}

func (s *Account) Register(ctx context.Context, in Credentials) (*Session, error) {
	if err := auth.ValidateUsername(in.Username); err != nil {
		return nil, apperr.WithCause(apperr.InvalidArgument, err)
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, apperr.WithCause(apperr.InvalidArgument, err)
	}
	id, err := s.users.CreateUser(ctx, in.Username, hash)
	if errors.Is(err, mysql.ErrDuplicateUsername) {
		return nil, apperr.WithCause(apperr.Conflict, err)
	}
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return s.session(entity.User{ID: id, Username: in.Username, Elo: 1000})
}

func (s *Account) Login(ctx context.Context, in Credentials) (*Session, error) {
	if in.Username == "" || len(in.Username) > 64 || len(in.Password) < 6 || len(in.Password) > 64 {
		return nil, apperr.BadCredentials
	}
	user, hash, err := s.users.GetUserByName(ctx, in.Username)
	if err != nil {
		return nil, fmt.Errorf("find login user: %w", err)
	}
	if user == nil || !auth.CheckPassword(hash, in.Password) {
		return nil, apperr.BadCredentials
	}
	return s.session(*user)
}

func (s *Account) session(user entity.User) (*Session, error) {
	token, err := s.auth.IssueToken(user.ID, user.Username)
	if err != nil {
		return nil, fmt.Errorf("issue token: %w", err)
	}
	return &Session{Token: token, User: user}, nil
}

func (s *Account) Profile(ctx context.Context, id int64) (*Profile, error) {
	user, err := s.users.GetUserByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	if user == nil {
		return nil, apperr.UserNotFound
	}
	recent, err := s.users.RecentGames(ctx, id, 10)
	if err != nil {
		return nil, fmt.Errorf("get recent games: %w", err)
	}
	if recent == nil {
		recent = []entity.GameRecord{}
	}
	return &Profile{User: *user, Recent: recent}, nil
}

func (s *Account) Leaderboard(ctx context.Context) (*Leaderboard, error) {
	users, err := s.users.TopPlayers(ctx, 20)
	if err != nil {
		return nil, fmt.Errorf("get leaderboard: %w", err)
	}
	if users == nil {
		users = []entity.User{}
	}
	return &Leaderboard{Players: users}, nil
}
