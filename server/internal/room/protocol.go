package room

import "penguin-chess/server/internal/game"

type msgRoom struct {
	Type string `json:"type"`
	Room string `json:"room"`
	You  int    `json:"you"`
}

type msgWaiting struct {
	Type string `json:"type"`
}

type msgStart struct {
	Type  string          `json:"type"`
	State *game.GameState `json:"state"`
	Names [2]string       `json:"names"`
}

type msgState struct {
	Type  string          `json:"type"`
	State *game.GameState `json:"state"`
	Last  *game.Move      `json:"last"`
}

type msgOver struct {
	Type   string          `json:"type"`
	State  *game.GameState `json:"state"`
	Winner int             `json:"winner"`
	Last   *game.Move      `json:"last"`
}

type msgSimple struct {
	Type string `json:"type"`
}

type msgError struct {
	Type      string `json:"type"`
	Msg       string `json:"msg"`
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
	Data      any    `json:"data"`
}

type inMsg struct {
	Type string `json:"type"`
	Room string `json:"room"`
	Tile int    `json:"tile"`
	From int    `json:"from"`
	To   int    `json:"to"`
}
