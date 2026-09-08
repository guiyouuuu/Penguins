package game

import "testing"

// 全流程对局：放置 → 移动 → 终局，验证状态机闭环
func TestFullGame(t *testing.T) {
	s := NewGame("A", "B")

	if len(s.Tiles) != 60 {
		t.Fatalf("棋盘格数 = %d, 期望 60", len(s.Tiles))
	}
	totalFish := 0
	counts := map[int]int{}
	for _, tile := range s.Tiles {
		totalFish += tile.Fish
		counts[tile.Fish]++
	}
	if totalFish != 100 || counts[1] != 30 || counts[2] != 20 || counts[3] != 10 {
		t.Fatalf("鱼分布错误: 总量=%d 1鱼=%d 2鱼=%d 3鱼=%d", totalFish, counts[1], counts[2], counts[3])
	}
	for _, p := range s.Players {
		if len(p.Penguins) != 4 {
			t.Fatalf("玩家 %d 企鹅数 = %d, 期望 4", p.ID, len(p.Penguins))
		}
		for _, pos := range p.Penguins {
			if pos != -1 {
				t.Fatalf("新对局企鹅应为未放置(-1), 实际 %d", pos)
			}
		}
	}

	// 放置 8 步
	ply := 0
	for s.Phase == PhasePlacing {
		moves := s.AllMoves(s.Turn)
		if len(moves) == 0 {
			t.Fatal("放置阶段无合法着法")
		}
		next, err := s.ApplyMove(&moves[0])
		if err != nil {
			t.Fatalf("第 %d 步放置失败: %v", ply+1, err)
		}
		s = next
		ply++
	}
	if ply != 8 {
		t.Fatalf("放置步数 = %d, 期望 8", ply)
	}
	if s.Phase != PhaseMoving {
		t.Fatalf("阶段 = %s, 期望 moving", s.Phase)
	}
	if s.Turn != 0 {
		t.Fatalf("移动阶段先手 = %d, 期望 0（先放置者先行动）", s.Turn)
	}
	// 每只企鹅都应有归属格
	for _, p := range s.Players {
		for i, pos := range p.Penguins {
			if pos < 0 {
				t.Fatalf("玩家 %d 第 %d 只企鹅未放置", p.ID, i)
			}
			if s.Tiles[pos].Owner != int(p.ID) {
				t.Fatalf("玩家 %d 企鹅位置 %d 与格子归属 %d 不一致", p.ID, pos, s.Tiles[pos].Owner)
			}
		}
	}

	// 移动直到终局
	guard := 0
	for s.Phase != PhaseFinished && guard < 600 {
		guard++
		moves := s.AllMoves(s.Turn)
		if len(moves) == 0 {
			t.Fatalf("第 %d 步：轮到玩家 %d 但无着法（死锁）", s.Ply, s.Turn)
		}
		next, err := s.ApplyMove(&moves[0])
		if err != nil {
			t.Fatalf("第 %d 步移动失败: %v", s.Ply, err)
		}
		s = next
	}
	if s.Phase != PhaseFinished {
		t.Fatal("对局未在合理步数内结束")
	}
	if s.Winner < -1 || s.Winner > 1 {
		t.Fatalf("胜者值非法: %d", s.Winner)
	}
	eaten := s.Players[0].Score + s.Players[1].Score
	if eaten > 100 {
		t.Fatalf("吃鱼总量 %d 超过 100", eaten)
	}
}

// 非法走子校验
func TestIllegalMoves(t *testing.T) {
	s := NewGame("A", "B")

	// 非当前玩家
	if _, err := s.ApplyMove(&Move{Kind: "place", Player: 1, From: -1, To: s.LegalPlacements()[0]}); err == nil {
		t.Fatal("非当前玩家走子应被拒绝")
	}
	// 2 鱼格不可放置
	twoFish := -1
	for i, tile := range s.Tiles {
		if tile.Fish == 2 {
			twoFish = i
			break
		}
	}
	if _, err := s.ApplyMove(&Move{Kind: "place", Player: 0, From: -1, To: twoFish}); err == nil {
		t.Fatal("2 鱼格放置应被拒绝")
	}
	// 移动阶段前不可 move
	if _, err := s.ApplyMove(&Move{Kind: "move", Player: 0, From: 0, To: 1}); err == nil {
		t.Fatal("放置阶段 move 应被拒绝")
	}
}

// 移动规则：滑行、阻挡、计分
func TestSlideAndScore(t *testing.T) {
	s := NewGame("A", "B")
	for i := range s.Tiles {
		s.Tiles[i].Fish = 1
	}
	// 手动放两只企鹅：格 0（A）和格 2（A 邻居右侧第二格）
	s.Phase = PhaseMoving
	s.Tiles[0].Owner = 0
	s.Tiles[2].Owner = 1
	s.Players[0].Penguins = []int{0}
	s.Players[1].Penguins = []int{2}
	s.Turn = 0
	s.Tiles[1].Fish = 3

	moves := s.LegalMovesFrom(0)
	if len(moves) == 0 {
		t.Fatal("格 0 应有滑行目标")
	}
	// 格 0 (q=0,r=0) 方向 (1,0)：格 1 (q=1,r=0) 空 → 合法；格 2 被企鹅占据 → 停
	found := false
	for _, m := range moves {
		if m == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("格 1 应为合法目标: %v", moves)
	}
	for _, m := range moves {
		if m == 2 {
			t.Fatal("被企鹅占据的格 2 不应为合法目标")
		}
	}

	// 执行 0 → 1
	next, err := s.ApplyMove(&Move{Kind: "move", Player: 0, From: 0, To: 1})
	if err != nil {
		t.Fatalf("移动失败: %v", err)
	}
	if next.Tiles[0].Gone != true || next.Tiles[0].Owner != -1 {
		t.Fatal("起点格应沉没且无归属")
	}
	if next.Players[0].Score != 3 {
		t.Fatalf("得分 = %d, 期望到达三鱼格立即获得 3 分", next.Players[0].Score)
	}
	if next.Players[0].Penguins[0] != 1 {
		t.Fatalf("企鹅位置 = %d, 期望 1", next.Players[0].Penguins[0])
	}
	if next.Turn != 1 {
		t.Fatalf("轮到玩家 %d, 期望 1", next.Turn)
	}
}

// AI 搜索不崩溃且速度可接受
func TestAIRun(t *testing.T) {
	s := NewGame("A", "B")
	ply := 0
	for s.Phase == PhasePlacing {
		moves := s.AllMoves(s.Turn)
		next, _ := s.ApplyMove(&moves[0])
		s = next
		ply++
	}
	guard := 0
	for s.Phase != PhaseFinished && guard < 600 {
		guard++
		moves := s.AllMoves(s.Turn)
		if len(moves) == 0 {
			t.Fatal("AI 测试死锁")
		}
		next, _ := s.ApplyMove(&moves[len(moves)-1])
		s = next
	}
}
