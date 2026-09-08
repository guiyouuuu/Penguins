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
		Players: []Player{{ID: 0, Penguins: []int{0}}, {ID: 1, Penguins: []int{second}}},
		Phase:   phase, Turn: 1, Winner: -2, Ply: 1,
	}
	if second >= 0 {
		s.Tiles[second].Owner = 1
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
	if before.Players[0].Score != 0 {
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
