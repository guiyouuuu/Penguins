// Package game 实现企鹅棋权威规则引擎（与前端 TS 引擎保持同一套规则）。
package game

import (
	"errors"
	"math/rand"
)

// Phase 对局阶段
type Phase string

const (
	PhasePlacing  Phase = "placing"
	PhaseMoving   Phase = "moving"
	PhaseFinished Phase = "finished"
)

// Tile 单个冰面格
type Tile struct {
	Q     int  `json:"q"`
	R     int  `json:"r"`
	Fish  int  `json:"fish"`
	Gone  bool `json:"gone"`
	Owner int  `json:"owner"` // -1 无企鹅
}

// Player 玩家
type Player struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	UserID   int64  `json:"userId"` // 数据库用户 ID，0 = 游客
	Score    int    `json:"score"`
	Penguins []int  `json:"penguins"` // -1 未放置
	Stuck    bool   `json:"stuck"`
}

// GameState 对局状态（与前端类型一一对应）
type GameState struct {
	Tiles   []Tile   `json:"tiles"`
	Players []Player `json:"players"`
	Phase   Phase    `json:"phase"`
	Turn    int      `json:"turn"`
	Winner  int      `json:"winner"` // -1 平局；-2 未结束
	Ply     int      `json:"ply"`
}

// Move 一步棋
type Move struct {
	Kind   string `json:"kind"` // place | move
	Player int    `json:"player"`
	From   int    `json:"from"`
	To     int    `json:"to"`
}

// hexDirs 六个滑行方向（axial 坐标）
var hexDirs = [6][2]int{
	{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1},
}

// 常见错误
var (
	ErrNotYourTurn = errors.New("还没轮到你")
	ErrGameOver    = errors.New("对局已结束")
	ErrIllegalMove = errors.New("不合法的走子")
	ErrBadPhase    = errors.New("当前阶段不能这样走")
)

// boardCoords 官方 60 格棋盘：8 行，偶数行 8 格、奇数行 7 格
func boardCoords() []struct{ Q, R int } {
	cells := make([]struct{ Q, R int }, 0, 60)
	for row := 0; row < 8; row++ {
		cols := 8
		if row%2 == 1 {
			cols = 7
		}
		for col := 0; col < cols; col++ {
			q := col - (row-(row&1))/2
			cells = append(cells, struct{ Q, R int }{q, row})
		}
	}
	return cells
}

// penguinCount 按人数返回企鹅数
func penguinCount(playerCount int) int {
	if playerCount == 2 {
		return 4
	}
	if playerCount == 3 {
		return 3
	}
	return 2
}

// NewGame 创建随机新对局（2 人局）
func NewGame(name0, name1 string) *GameState {
	coords := boardCoords()
	// 鱼：30 格 1 鱼、20 格 2 鱼、10 格 3 鱼
	bag := make([]int, 0, 60)
	for i := 0; i < 30; i++ {
		bag = append(bag, 1)
	}
	for i := 0; i < 20; i++ {
		bag = append(bag, 2)
	}
	for i := 0; i < 10; i++ {
		bag = append(bag, 3)
	}
	rand.Shuffle(len(bag), func(i, j int) { bag[i], bag[j] = bag[j], bag[i] })

	tiles := make([]Tile, 60)
	for i, c := range coords {
		tiles[i] = Tile{Q: c.Q, R: c.R, Fish: bag[i], Owner: -1}
	}
	n := penguinCount(2)
	peng0 := make([]int, n)
	peng1 := make([]int, n)
	for i := range peng0 {
		peng0[i] = -1
		peng1[i] = -1
	}
	return &GameState{
		Tiles: tiles,
		Players: []Player{
			{ID: 0, Name: name0, Penguins: peng0, Score: 0},
			{ID: 1, Name: name1, Penguins: peng1, Score: 0},
		},
		Phase:  PhasePlacing,
		Turn:   0,
		Winner: -2,
	}
}

// Clone 深拷贝（AI 搜索用）
func (s *GameState) Clone() *GameState {
	tiles := make([]Tile, len(s.Tiles))
	copy(tiles, s.Tiles)
	players := make([]Player, len(s.Players))
	for i, p := range s.Players {
		peng := make([]int, len(p.Penguins))
		copy(peng, p.Penguins)
		players[i] = p
		players[i].Penguins = peng
	}
	return &GameState{
		Tiles:   tiles,
		Players: players,
		Phase:   s.Phase,
		Turn:    s.Turn,
		Winner:  s.Winner,
		Ply:     s.Ply,
	}
}

// find 按 axial 坐标找格索引，-1 不存在
func (s *GameState) find(q, r int) int {
	for i := range s.Tiles {
		if s.Tiles[i].Q == q && s.Tiles[i].R == r {
			return i
		}
	}
	return -1
}

// LegalPlacements 放置阶段合法格：未沉没、无企鹅、恰好 1 鱼
func (s *GameState) LegalPlacements() []int {
	res := make([]int, 0, 32)
	for i := range s.Tiles {
		t := &s.Tiles[i]
		if !t.Gone && t.Owner == -1 && t.Fish == 1 {
			res = append(res, i)
		}
	}
	return res
}

// LegalMovesFrom 某格企鹅的滑行目标：六方向直线，遇洞 / 企鹅 / 边界停止
func (s *GameState) LegalMovesFrom(idx int) []int {
	if idx < 0 || idx >= len(s.Tiles) {
		return nil
	}
	t := &s.Tiles[idx]
	res := make([]int, 0, 8)
	for _, d := range hexDirs {
		q, r := t.Q+d[0], t.R+d[1]
		for {
			n := s.find(q, r)
			if n == -1 {
				break
			}
			nt := &s.Tiles[n]
			if nt.Gone || nt.Owner != -1 {
				break
			}
			res = append(res, n)
			q += d[0]
			r += d[1]
		}
	}
	return res
}

// PlayerCanMove 玩家是否还有可行动的企鹅
func (s *GameState) PlayerCanMove(player int) bool {
	for _, idx := range s.Players[player].Penguins {
		if idx >= 0 && len(s.LegalMovesFrom(idx)) > 0 {
			return true
		}
	}
	return false
}

// AllMoves 玩家当前全部合法着法
func (s *GameState) AllMoves(player int) []Move {
	res := make([]Move, 0, 16)
	if s.Phase == PhaseFinished || s.Turn != player {
		return res
	}
	if s.Phase == PhasePlacing {
		for _, i := range s.LegalPlacements() {
			res = append(res, Move{Kind: "place", Player: player, From: -1, To: i})
		}
		return res
	}
	for _, idx := range s.Players[player].Penguins {
		if idx < 0 {
			continue
		}
		for _, to := range s.LegalMovesFrom(idx) {
			res = append(res, Move{Kind: "move", Player: player, From: idx, To: to})
		}
	}
	return res
}

// finishGame 终局结算
func (s *GameState) finishGame() {
	s.Phase = PhaseFinished
	best, bestScore, tie := -1, -1, false
	for _, p := range s.Players {
		if p.Score > bestScore {
			bestScore, best, tie = p.Score, int(p.ID), false
		} else if p.Score == bestScore {
			tie = true
		}
	}
	if tie {
		s.Winner = -1
	} else {
		s.Winner = best
	}
}

// eliminatePlayer 淘汰玩家（官方规则）：全部企鹅被困 → 企鹅离场，带走脚下格子的鱼，格子沉没
func (s *GameState) eliminatePlayer(player int) {
	p := &s.Players[player]
	for i, idx := range p.Penguins {
		if idx >= 0 {
			t := &s.Tiles[idx]
			t.Owner = -1
			t.Gone = true
			p.Score += t.Fish
			p.Penguins[i] = -1
		}
	}
	p.Stuck = true
}

// advanceTurn 推进行动权：出局玩家跳过；无人可动则终局
func (s *GameState) advanceTurn() {
	n := len(s.Players)
	for step := 1; step <= n; step++ {
		next := (s.Turn + step) % n
		if s.Players[next].Stuck {
			continue // 已出局
		}
		if s.PlayerCanMove(next) {
			s.Turn = next
			return
		}
		s.eliminatePlayer(next)
	}
	s.finishGame()
}

// ApplyMove 校验并执行一步棋（返回新状态，原状态不变）
func (s *GameState) ApplyMove(mv *Move) (*GameState, error) {
	if s.Phase == PhaseFinished {
		return nil, ErrGameOver
	}
	if mv.Player != s.Turn {
		return nil, ErrNotYourTurn
	}
	next := s.Clone()
	if err := next.applyChecked(mv); err != nil {
		return nil, err
	}
	return next, nil
}

// ApplyUnchecked 跳过回合校验直接应用（仅用于已确认合法的走法，如 AI 搜索）
func (s *GameState) ApplyUnchecked(mv *Move) error {
	if s.Phase == PhaseFinished {
		return ErrGameOver
	}
	return s.applyChecked(mv)
}

// applyChecked 已通过回合校验后的规则校验与应用
func (s *GameState) applyChecked(mv *Move) error {
	if mv.Kind == "place" {
		if s.Phase != PhasePlacing {
			return ErrBadPhase
		}
		if mv.To < 0 || mv.To >= len(s.Tiles) {
			return ErrIllegalMove
		}
		t := &s.Tiles[mv.To]
		if t.Gone || t.Owner != -1 || t.Fish != 1 {
			return ErrIllegalMove
		}
		t.Owner = mv.Player
		p := &s.Players[mv.Player]
		for i, pos := range p.Penguins {
			if pos == -1 {
				p.Penguins[i] = mv.To
				break
			}
		}
		s.Ply++
		total := 0
		for _, pl := range s.Players {
			total += len(pl.Penguins)
		}
		if s.Ply >= total {
			s.Phase = PhaseMoving
			// 移动阶段由第一个放置的玩家（玩家 0）先行动
			s.Turn = 0
			if !s.PlayerCanMove(s.Turn) {
				s.eliminatePlayer(s.Turn)
				s.advanceTurn()
			}
		} else {
			s.Turn = (s.Turn + 1) % len(s.Players)
		}
		return nil
	}

	// move
	if s.Phase != PhaseMoving {
		return ErrBadPhase
	}
	if mv.From < 0 || mv.From >= len(s.Tiles) || mv.To < 0 || mv.To >= len(s.Tiles) {
		return ErrIllegalMove
	}
	from := &s.Tiles[mv.From]
	if from.Owner != mv.Player {
		return ErrIllegalMove
	}
	legal := false
	for _, to := range s.LegalMovesFrom(mv.From) {
		if to == mv.To {
			legal = true
			break
		}
	}
	if !legal {
		return ErrIllegalMove
	}
	from.Owner = -1
	from.Gone = true
	s.Tiles[mv.To].Owner = mv.Player
	s.Players[mv.Player].Score += from.Fish
	p := &s.Players[mv.Player]
	for i, pos := range p.Penguins {
		if pos == mv.From {
			p.Penguins[i] = mv.To
			break
		}
	}
	s.Ply++
	// 自己走完即被困 → 立即出局（官方规则：企鹅离场并带走脚下的鱼）
	if !s.PlayerCanMove(mv.Player) {
		s.eliminatePlayer(mv.Player)
	}
	s.advanceTurn()
	return nil
}
