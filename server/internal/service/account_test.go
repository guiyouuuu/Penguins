package service

import (
	"context"
	"errors"
	"testing"

	"penguin-chess/server/model/entity"

	"penguin-chess/server/internal/repository/mysql"
	"penguin-chess/server/pkg/auth"
	apperr "penguin-chess/server/pkg/errors"
)

type fakeUsers struct {
	err        error
	historyErr error
	user       *entity.User
}

func (f fakeUsers) CreateUser(context.Context, string, string) (int64, error) { return 1, f.err }
func (f fakeUsers) GetUserByName(context.Context, string) (*entity.User, string, error) {
	return f.user, "", f.err
}
func (f fakeUsers) GetUserByID(context.Context, int64) (*entity.User, error) { return f.user, f.err }
func (f fakeUsers) RecentGames(context.Context, int64, int) ([]entity.GameRecord, error) {
	return nil, f.historyErr
}
func (f fakeUsers) TopPlayers(context.Context, int) ([]entity.User, error) { return nil, f.err }

func TestProfileDistinguishesDatabaseFailureFromMissingUser(t *testing.T) {
	for _, tc := range []struct {
		repo fakeUsers
		want *apperr.Error
	}{
		{fakeUsers{err: errors.New("database failure")}, apperr.Internal},
		{fakeUsers{}, apperr.UserNotFound},
		{fakeUsers{user: &entity.User{ID: 1}, historyErr: errors.New("history failure")}, apperr.Internal},
	} {
		svc := NewAccount(tc.repo, auth.New("test-secret"))
		_, err := svc.Profile(context.Background(), 1)
		if !errors.Is(apperr.Resolve(err), tc.want) {
			t.Fatalf("got %v, want %v", err, tc.want)
		}
	}
}

func TestRegisterMapsTypedDuplicate(t *testing.T) {
	svc := NewAccount(fakeUsers{err: mysql.ErrDuplicateUsername}, auth.New("test-secret"))
	_, err := svc.Register(context.Background(), Credentials{Username: "tester", Password: "password"})
	if !errors.Is(err, apperr.Conflict) {
		t.Fatalf("unexpected duplicate error %v", err)
	}
}
