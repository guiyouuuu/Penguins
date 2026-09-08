// Package room 房间管理：状态外置 Redis（分布式锁 + Pub/Sub 广播），支持多实例水平扩展。
package room

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	apperr "penguin-chess/server/pkg/errors"

	"github.com/redis/go-redis/v9"

	"penguin-chess/server/internal/game"
)

// Redis key 前缀与常量
const (
	keyRoomMeta   = "penguin:room:%s:meta"  // HASH：房间元数据
	keyRoomState  = "penguin:room:%s:state" // STRING：GameState JSON
	keyRoomLock   = "penguin:room:%s:lock"  // 分布式锁
	keyMatchQueue = "penguin:match:queue"   // ZSET：匹配队列
	keyMatchLock  = "penguin:match:lock"    // 匹配配对锁
	keyRoomIndex  = "penguin:rooms"         // SET：活跃房间索引
	chanBroadcast = "penguin:broadcast"     // 房间广播频道
	roomTTL       = 24 * time.Hour
	lockTTL       = 5 * time.Second
	matchLockTTL  = 3 * time.Second
	roomCodeChars = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
)

// RoomMeta 房间元数据（Redis HASH）
type RoomMeta struct {
	Mode       string `json:"mode"`      // pvp | ai
	Seat0UID   int64  `json:"seat0_uid"` // 0 = 游客
	Seat0Name  string `json:"seat0_name"`
	Seat1UID   int64  `json:"seat1_uid"`
	Seat1Name  string `json:"seat1_name"`
	Full       bool   `json:"full"`
	Started    bool   `json:"started"`
	GuestReady bool   `json:"guest_ready"`
}

// BroadCastMsg Pub/Sub 广播消息（实例间协调）
type BroadCastMsg struct {
	RoomID     string          `json:"room_id"`               // 房间
	Msg        json.RawMessage `json:"msg,omitempty"`         // 转发给客户端的消息
	TargetConn string          `json:"target_conn,omitempty"` // 定向连接（跨实例路由）
	BindSeat   int             `json:"bind_seat,omitempty"`   // 绑定席位（-1 无）
	LeaveSeat  int             `json:"leave_seat,omitempty"`  // 离房席位（-1 无）
	KeepRoom   bool            `json:"keep_room,omitempty"`   // 开局前客人离开，保留房主
}

// 常见错误
var (
	errRoomGone     = apperr.RoomGone
	errNotSeatOwner = apperr.Forbidden
)

// metaKey 房间元数据 key
func metaKey(roomID string) string {
	return fmt.Sprintf(keyRoomMeta, roomID)
}

// RoomStore Redis 房间存储层
type RoomStore struct {
	rdb *redis.Client
	ctx context.Context
}

// NewRoomStore 创建
func NewRoomStore(ctx context.Context, rdb *redis.Client) *RoomStore {
	return &RoomStore{rdb: rdb, ctx: ctx}
}

// genRoomCode 生成 4 位房间号（全局唯一）
func (s *RoomStore) genRoomCode() (string, error) {
	for attempt := 0; attempt < 50; attempt++ {
		b := make([]byte, 4)
		for i := range b {
			b[i] = roomCodeChars[rand.Intn(len(roomCodeChars))]
		}
		code := string(b)
		// SETNX 到房间索引实现原子占位
		added, err := s.rdb.SAdd(s.ctx, keyRoomIndex, code).Result()
		if err != nil {
			return "", err
		}
		if added == 1 {
			return code, nil
		}
	}
	return "", apperr.Unavailable
}

// CreateRoom 创建房间（返回房间号）
func (s *RoomStore) CreateRoom(mode string, uid int64, name string) (string, error) {
	code, err := s.genRoomCode()
	if err != nil {
		return "", err
	}
	meta := RoomMeta{Mode: mode, Seat0UID: uid, Seat0Name: name, Started: mode == "ai"}
	state := newGameFor(mode, uid, name)
	if err := s.saveMeta(code, &meta); err != nil {
		return "", err
	}
	if err := s.saveState(code, state); err != nil {
		return "", err
	}
	return code, nil
}

// newGameFor 按模式创建初始对局
func newGameFor(mode string, uid int64, name string) *game.GameState {
	var st *game.GameState
	if mode == "ai" {
		st = game.NewGame(name, "冰原企鹅")
	} else {
		st = game.NewGame(name, "对手企鹅")
	}
	st.Players[0].UserID = uid
	return st
}

// GetMeta 读取房间元数据；不存在返回 nil
func (s *RoomStore) GetMeta(roomID string) (*RoomMeta, error) {
	data, err := s.rdb.HGetAll(s.ctx, metaKey(roomID)).Result()
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	m := &RoomMeta{Mode: data["mode"]}
	m.Seat0UID = parseInt(data["seat0_uid"])
	m.Seat0Name = data["seat0_name"]
	m.Seat1UID = parseInt(data["seat1_uid"])
	m.Seat1Name = data["seat1_name"]
	m.Full = data["full"] == "1"
	m.Started = data["started"] == "1"
	m.GuestReady = data["guest_ready"] == "1"
	return m, nil
}

func parseInt(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

// saveMeta 写元数据（并刷新 TTL）
func (s *RoomStore) saveMeta(roomID string, m *RoomMeta) error {
	key := fmt.Sprintf(keyRoomMeta, roomID)
	fields := map[string]any{
		"mode":        m.Mode,
		"seat0_uid":   m.Seat0UID,
		"seat0_name":  m.Seat0Name,
		"seat1_uid":   m.Seat1UID,
		"seat1_name":  m.Seat1Name,
		"full":        m.Full,
		"started":     m.Started,
		"guest_ready": m.GuestReady,
	}
	pipe := s.rdb.TxPipeline()
	pipe.HSet(s.ctx, key, fields)
	pipe.Expire(s.ctx, key, roomTTL)
	_, err := pipe.Exec(s.ctx)
	return err
}

// JoinSeat1 第二位玩家加入
func (s *RoomStore) JoinSeat1(roomID string, uid int64, name string) error {
	key := fmt.Sprintf(keyRoomMeta, roomID)
	// 原子检查并占用 seat1：房间存在且未被占满时，写入席位并置 full=1
	ok, err := s.rdb.Eval(s.ctx, `
		if redis.call('HLEN', KEYS[1]) == 0 then
			return 0
		end
		if redis.call('HGET', KEYS[1], 'mode') ~= 'pvp' then return 0 end
		if redis.call('HGET', KEYS[1], 'seat0_uid') == ARGV[1] then return -1 end
		if redis.call('HGET', KEYS[1], 'started') == '1' then return -2 end
		if redis.call('HGET', KEYS[1], 'full') == '1' then
			return 0
		end
		redis.call('HSET', KEYS[1], 'full', '1', 'seat1_uid', ARGV[1], 'seat1_name', ARGV[2], 'guest_ready', '0')
		redis.call('EXPIRE', KEYS[1], ARGV[3])
		return 1
	`, []string{key}, fmt.Sprintf("%d", uid), name, fmt.Sprintf("%d", int64(roomTTL.Seconds()))).Int()
	if err != nil {
		return err
	}
	if ok == -1 {
		return apperr.SelfJoin
	}
	if ok == -2 {
		return apperr.RoomStarted
	}
	if ok != 1 {
		return apperr.RoomFull
	}
	return nil
}

// GetState 读取对局状态
func (s *RoomStore) GetState(roomID string) (*game.GameState, error) {
	data, err := s.rdb.Get(s.ctx, fmt.Sprintf(keyRoomState, roomID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var st game.GameState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// saveState 写对局状态（并刷新 TTL）
func (s *RoomStore) saveState(roomID string, st *game.GameState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	key := fmt.Sprintf(keyRoomState, roomID)
	pipe := s.rdb.TxPipeline()
	pipe.Set(s.ctx, key, data, roomTTL)
	_, err = pipe.Exec(s.ctx)
	return err
}

// withLock 房间级分布式锁（SETNX + 随机 token，Lua 安全释放；短暂重试缓解并发冲突）
func (s *RoomStore) withLock(roomID string, fn func() error) error {
	return s.withLockContext(s.ctx, roomID, fn)
}

func (s *RoomStore) withLockContext(ctx context.Context, roomID string, fn func() error) error {
	lockKey := fmt.Sprintf(keyRoomLock, roomID)
	token := fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Int63())
	acquired := false
	for i := 0; i < 10; i++ {
		ok, err := s.rdb.SetNX(ctx, lockKey, token, lockTTL).Result()
		if err != nil {
			return fmt.Errorf("获取房间锁失败: %w", err)
		}
		if ok {
			acquired = true
			break
		}
		if !waitFor(ctx, 20*time.Millisecond) {
			return ctx.Err()
		}
	}
	if !acquired {
		return apperr.RateLimited
	}
	defer s.rdb.Eval(ctx, `
		if redis.call('GET', KEYS[1]) == ARGV[1] then
			return redis.call('DEL', KEYS[1])
		end
		return 0
	`, []string{lockKey}, token)
	return fn()
}

// Publish 广播房间消息到所有实例
func (s *RoomStore) Publish(roomID string, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(BroadCastMsg{RoomID: roomID, Msg: data, BindSeat: -1, LeaveSeat: -1})
	if err != nil {
		return err
	}
	return s.rdb.Publish(s.ctx, chanBroadcast, payload).Err()
}

// CloseRoom 删除房间所有键
func (s *RoomStore) CloseRoom(roomID string) error {
	return s.CloseRoomContext(s.ctx, roomID)
}

func (s *RoomStore) CloseRoomContext(ctx context.Context, roomID string) error {
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, fmt.Sprintf(keyRoomMeta, roomID))
	pipe.Del(ctx, fmt.Sprintf(keyRoomState, roomID))
	pipe.SRem(ctx, keyRoomIndex, roomID)
	_, err := pipe.Exec(ctx)
	return err
}

// ResetForRematch 再来一局：重置状态与 full 标记（保留席位）
func (s *RoomStore) ResetForRematch(roomID string, m *RoomMeta) (*game.GameState, error) {
	st := game.NewGame(m.Seat0Name, m.Seat1Name)
	st.Players[0].UserID = m.Seat0UID
	st.Players[1].UserID = m.Seat1UID
	if err := s.saveState(roomID, st); err != nil {
		return nil, err
	}
	return st, nil
}
