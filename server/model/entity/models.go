package entity

import "time"

// User 用户档案
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Wins     int    `json:"wins"`
	Losses   int    `json:"losses"`
	Draws    int    `json:"draws"`
	Elo      int    `json:"elo"`
}

// GameResult 一局结束后的战绩结算
type GameResult struct {
	RoomID       string
	Player0ID    int64
	Player1ID    int64
	Player0Name  string
	Player1Name  string
	Player0Score int
	Player1Score int
	Winner       int // 0 / 1 / -1 平局
	TotalPly     int
}

// RecentGames 玩家最近对局
type GameRecord struct {
	ID         int64     `json:"id"`
	RoomID     string    `json:"room_id"`
	Opponent   string    `json:"opponent"`
	MyScore    int       `json:"my_score"`
	OppScore   int       `json:"opp_score"`
	Winner     int       `json:"winner"`
	EloDelta   int       `json:"elo_delta"`
	FinishedAt time.Time `json:"finished_at"`
}
