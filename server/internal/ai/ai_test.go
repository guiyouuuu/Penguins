package ai

import (
	"testing"

	"penguin-chess/server/internal/game"
)

// AI 自对弈完整对局：不崩溃、步数合理、能分出胜负
func TestSelfPlay(t *testing.T) {
	s := game.NewGame("AI-0", "AI-1")
	guard := 0
	for s.Phase != game.PhaseFinished && guard < 600 {
		guard++
		mv := ChooseMove(s, s.Turn)
		if mv == nil {
			t.Fatalf("第 %d 步：AI 无着法但对局未结束（死锁）", s.Ply)
		}
		next, err := s.ApplyMove(mv)
		if err != nil {
			t.Fatalf("第 %d 步：AI 走子被拒: %v", s.Ply, err)
		}
		s = next
	}
	if s.Phase != game.PhaseFinished {
		t.Fatalf("对局未结束 (%d 步)", guard)
	}
	if s.Winner < -1 || s.Winner > 1 {
		t.Fatalf("胜者非法: %d", s.Winner)
	}
	t.Logf("自对弈 %d 步结束，胜者 %d，比分 %d:%d", s.Ply, s.Winner, s.Players[0].Score, s.Players[1].Score)
}
