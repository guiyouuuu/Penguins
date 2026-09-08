package room

import (
	"errors"

	"penguin-chess/server/internal/game"
	apperr "penguin-chess/server/pkg/errors"
)

func publicError(err error) *apperr.Error {
	for _, pair := range []struct {
		domain error
		public *apperr.Error
	}{
		{game.ErrNotYourTurn, apperr.NotYourTurn}, {game.ErrGameOver, apperr.GameOver},
		{game.ErrIllegalMove, apperr.IllegalMove}, {game.ErrBadPhase, apperr.BadPhase},
	} {
		if errors.Is(err, pair.domain) {
			return apperr.WithCause(pair.public, err)
		}
	}
	return apperr.Resolve(err)
}
