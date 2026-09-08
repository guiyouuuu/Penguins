// Package ai 企鹅棋 AI：negamax + α-β 剪枝，评估 = 分差 + 可达鱼 + 灵活度。
package ai

import (
	"math"
	"math/rand"

	"penguin-chess/server/internal/game"
)

const (
	searchDepth    = 3
	weightScore    = 12.0
	weightReach    = 2.0
	weightMobility = 0.4
)

// ChooseMove 为 player 挑选一步棋；无棋可走返回 nil
func ChooseMove(s *game.GameState, player int) *game.Move {
	moves := s.AllMoves(player)
	if len(moves) == 0 {
		return nil
	}
	if s.Phase == game.PhasePlacing {
		return choosePlacement(s, moves, player)
	}
	return searchRoot(s, moves, player)
}

// choosePlacement 放置阶段启发式：优先落在「未来可达鱼」多的位置，带随机扰动
func choosePlacement(s *game.GameState, moves []game.Move, player int) *game.Move {
	best := moves[0]
	bestVal := math.Inf(-1)
	for _, mv := range moves {
		sim := s.Clone()
		_ = sim.ApplyUnchecked(&mv)
		val := float64(reachableFish(sim, player)) + rand.Float64()*1.5
		if val > bestVal {
			bestVal = val
			best = mv
		}
	}
	m := best
	return &m
}

// searchRoot 移动阶段根节点搜索
func searchRoot(s *game.GameState, moves []game.Move, player int) *game.Move {
	best := moves[0]
	bestScore := math.Inf(-1)
	alpha, beta := math.Inf(-1), math.Inf(1)
	for _, mv := range moves {
		ns := s.Clone()
		_ = ns.ApplyUnchecked(&mv)
		// negamax：子节点以对手视角评估
		score := -negamax(ns, 1-player, searchDepth-1, -beta, -alpha)
		if score > bestScore {
			bestScore = score
			best = mv
		}
		if score > alpha {
			alpha = score
		}
	}
	m := best
	return &m
}

// negamax 负极大值搜索，返回当前行动方视角的分值
func negamax(s *game.GameState, player int, depth int, alpha, beta float64) float64 {
	if s.Phase == game.PhaseFinished || depth == 0 {
		return evaluate(s, player)
	}
	moves := s.AllMoves(player)
	if len(moves) == 0 {
		return evaluate(s, player)
	}
	best := math.Inf(-1)
	for _, mv := range moves {
		ns := s.Clone()
		_ = ns.ApplyUnchecked(&mv)
		score := -negamax(ns, 1-player, depth-1, -beta, -alpha)
		if score > best {
			best = score
		}
		if best > alpha {
			alpha = best
		}
		if alpha >= beta {
			break // β 剪枝
		}
	}
	return best
}

// evaluate 评估函数：player 视角
func evaluate(s *game.GameState, player int) float64 {
	opp := 1 - player
	if s.Phase == game.PhaseFinished {
		diff := float64(s.Players[player].Score - s.Players[opp].Score)
		if diff > 0 {
			return 10000 + diff
		}
		if diff < 0 {
			return -10000 + diff
		}
		return 0
	}
	scoreDiff := float64(s.Players[player].Score - s.Players[opp].Score)
	reachDiff := float64(reachableFish(s, player) - reachableFish(s, opp))
	mobDiff := float64(len(s.AllMoves(player)) - len(s.AllMoves(opp)))
	return scoreDiff*weightScore + reachDiff*weightReach + mobDiff*weightMobility
}

// reachableFish 玩家全部企鹅可达区域的鱼总量（传递闭包，重叠去重）
func reachableFish(s *game.GameState, player int) int {
	set := make(map[int]bool)
	for _, pos := range s.Players[player].Penguins {
		if pos < 0 {
			continue
		}
		floodReach(s, pos, set)
	}
	total := 0
	for i := range set {
		total += s.Tiles[i].Fish
	}
	return total
}

// floodReach 从 from 出发的滑行传递闭包（忽略 from 自身阻挡的近似）
func floodReach(s *game.GameState, from int, set map[int]bool) {
	queue := make([]int, 0, 16)
	for _, to := range s.LegalMovesFrom(from) {
		if !set[to] {
			set[to] = true
			queue = append(queue, to)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, to := range s.LegalMovesFrom(cur) {
			if !set[to] {
				set[to] = true
				queue = append(queue, to)
			}
		}
	}
}
