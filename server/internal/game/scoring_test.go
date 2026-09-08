package game

import (
	"math/rand"
	"testing"
)

func TestPlacementCollectsFishImmediately(t *testing.T) {
	before := NewGame("A", "B")
	to := before.LegalPlacements()[0]
	next, err := before.ApplyMove(&Move{Kind: "place", Player: 0, From: -1, To: to})
	if err != nil {
		t.Fatal(err)
	}
	if next.Players[0].Score != 1 || before.Players[0].Score != 0 {
		t.Fatal("placement must score immediately without changing input")
	}
}

func TestArrivalScoringWithoutDuplicateDepartureOrRemoval(t *testing.T) {
	for _, unchecked := range []bool{false, true} {
		before := &GameState{
			Tiles:   []Tile{{Q: 0, Fish: 1, Owner: 0}, {Q: 1, Fish: 3, Owner: -1}, {Q: 2, Fish: 2, Owner: -1}, {Q: 3, Fish: 1, Owner: -1}},
			Players: []Player{{ID: 0, Score: 1, Penguins: []int{0}}, {ID: 1, Penguins: []int{-1}, Stuck: true}},
			Phase:   PhaseMoving, Turn: 0, Winner: -2,
		}
		next := before.Clone()
		for _, step := range []struct{ from, to, score int }{{0, 1, 4}, {1, 2, 6}, {2, 3, 7}} {
			move := &Move{Kind: "move", Player: 0, From: step.from, To: step.to}
			var err error
			if unchecked {
				err = next.ApplyUnchecked(move)
			} else {
				next, err = next.ApplyMove(move)
			}
			if err != nil {
				t.Fatal(err)
			}
			if next.Players[0].Score != step.score {
				t.Fatalf("unchecked=%v, move=%+v: score=%d, want=%d", unchecked, move, next.Players[0].Score, step.score)
			}
		}
		if next.Phase != PhaseFinished || next.Players[0].Penguins[0] != -1 || before.Players[0].Score != 1 {
			t.Fatal("incorrect finish or mutated input")
		}
	}
}

func TestScoreConservationThroughoutGames(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for round := 0; round < 20; round++ {
		s := NewGame("A", "B")
		for steps := 0; s.Phase != PhaseFinished; steps++ {
			if steps >= 100 {
				t.Fatal("game did not finish")
			}
			moves := s.AllMoves(s.Turn)
			if len(moves) == 0 {
				t.Fatal("no legal move for active player")
			}
			move := moves[rng.Intn(len(moves))]
			var err error
			s, err = s.ApplyMove(&move)
			if err != nil {
				t.Fatal(err)
			}
			collected := 0
			for _, tile := range s.Tiles {
				if tile.Gone || tile.Owner >= 0 {
					collected += tile.Fish
				}
			}
			if s.Players[0].Score+s.Players[1].Score != collected {
				t.Fatal("scores do not match fish on visited ice")
			}
		}
	}
}
