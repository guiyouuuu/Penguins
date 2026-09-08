package game

import "testing"

func isolatedGame(phase Phase) *GameState {

	second := 1
	if phase == PhasePlacing {
		second = -1
	}
	s := &GameState{
		Tiles: []Tile{
			{Q: 0, Fish: 1, Owner: 0}, {Q: 5, Fish: 1, Owner: -1},
			{Q: 6, Fish: 1, Owner: -1}, {Q: 7, Fish: 1, Owner: -1},
		},
		Players: []Player{{ID: 0, Score: 1, Penguins: []int{0}}, {ID: 1, Penguins: []int{second}}},
		Phase:   phase, Turn: 1, Winner: -2, Ply: 1,
	}
	if second >= 0 {
		s.Tiles[second].Owner = 1
		s.Players[1].Score = 1
	}
	return s
}

func TestBlockedFirstPlayerAfterPlacement(t *testing.T) {
	before := isolatedGame(PhasePlacing)
	next, err := before.ApplyMove(&Move{Kind: "place", Player: 1, From: -1, To: 1})
	if err != nil {
		t.Fatal(err)
	}
	if next.Phase != PhaseMoving || next.Turn != 1 || !next.Players[0].Stuck || next.Players[0].Score != 1 || !next.Tiles[0].Gone || next.Players[0].Penguins[0] != -1 {
		t.Fatalf("blocked first player must be settled before opponent continues: %+v", next)
	}
	if before.Players[0].Score != 1 {
		t.Fatal("input state mutated")
	}
}

func TestOpponentContinuesAlone(t *testing.T) {
	next, err := isolatedGame(PhaseMoving).ApplyMove(&Move{Kind: "move", Player: 1, From: 1, To: 2})
	if err != nil {
		t.Fatal(err)
	}
	if next.Phase != PhaseMoving || next.Turn != 1 || !next.Players[0].Stuck || next.Players[0].Score != 1 {
		t.Fatalf("opponent must continue after elimination: %+v", next)
	}
	end, err := next.ApplyMove(&Move{Kind: "move", Player: 1, From: 2, To: 3})
	if err != nil {
		t.Fatal(err)
	}
	if end.Phase != PhaseFinished || end.Winner != 1 || end.Players[0].Score != 1 || end.Players[1].Score != 3 {
		t.Fatalf("incorrect final settlement: %+v", end)
	}
}

func TestSingleIcePenguinFallsWhileOthersCanMove(t *testing.T) {
	for _, mover := range []int{0, 1} {
		t.Run([]string{"moving penguin", "opponent penguin"}[mover], func(t *testing.T) {
			s := &GameState{
				Tiles:   []Tile{{Q: 0, Fish: 3, Owner: 0}, {Q: 1, Fish: 2, Owner: 1}, {Q: 2, Fish: 1, Owner: -1}, {Q: 3, Fish: 1, Owner: -1}, {Q: 10, Fish: 1, Owner: 0}, {Q: 11, Fish: 1, Owner: -1}, {Q: 12, Fish: 1, Owner: 1}},
				Players: []Player{{ID: 0, Score: 4, Penguins: []int{0, 4}}, {ID: 1, Score: 3, Penguins: []int{1, 6}}},
				Phase:   PhaseMoving, Turn: mover, Winner: -2,
			}
			move := &Move{Kind: "move", Player: mover, From: 1, To: 2}
			wantScore := 4
			if mover == 0 {
				s.Tiles[0].Owner, s.Tiles[1].Owner = -1, 0
				s.Players[0].Penguins[0], s.Players[1].Penguins[0] = 1, -1
				s.Players[0].Score, s.Players[1].Score = 3, 1
				move.To, wantScore = 0, 6
			}
			next, err := s.ApplyMove(move)
			if err != nil {
				t.Fatal(err)
			}
			if next.Players[0].Penguins[0] != -1 || !next.Tiles[0].Gone || next.Tiles[0].Owner != -1 || next.Players[0].Score != wantScore {
				t.Fatalf("isolated penguin was not settled: %+v", next)
			}
			if next.Players[0].Stuck || next.Players[0].Penguins[1] != 4 || !next.PlayerCanMove(0) || next.Phase != PhaseMoving {
				t.Fatal("remaining mobile penguin must stay in play")
			}
			if s.Players[0].Score != []int{3, 4}[mover] || s.Tiles[0].Gone {
				t.Fatal("input mutated")
			}
		})
	}
}

func TestOccupiedAdjacentIceDoesNotCountAsSingleIce(t *testing.T) {
	s := &GameState{
		Tiles:   []Tile{{Q: 0, Fish: 3, Owner: 0}, {Q: 1, Fish: 1, Owner: 1}, {Q: 2, Fish: 1, Owner: -1}, {Q: 5, Fish: 1, Owner: 0}, {Q: 6, Fish: 1, Owner: -1}, {Q: 10, Fish: 1, Owner: 1}, {Q: 11, Fish: 1, Owner: -1}, {Q: 12, Fish: 1, Owner: -1}},
		Players: []Player{{ID: 0, Penguins: []int{0, 3}}, {ID: 1, Penguins: []int{1, 5}}},
		Phase:   PhaseMoving, Turn: 1, Winner: -2,
	}
	next, err := s.ApplyMove(&Move{Kind: "move", Player: 1, From: 5, To: 6})
	if err != nil {
		t.Fatal(err)
	}
	if next.Tiles[0].Gone || next.Players[0].Penguins[0] != 0 || next.Players[0].Score != 0 {
		t.Fatal("connected ice must not collapse solely because its neighbor is occupied")
	}
}
