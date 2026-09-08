// Package errors 定义跨 HTTP / WebSocket 的稳定错误码和公开消息。
package errors

import "errors"

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
	cause   error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.cause }
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

// 错误值视为不可变；添加上下文时复制，不修改全局定义。
func WithCause(kind *Error, cause error) *Error {
	e := *kind
	e.cause = cause
	return &e
}

func Resolve(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return WithCause(Internal, err)
}

var (
	InvalidArgument  = &Error{Code: 10001, Message: "请求参数不合法", Status: 400}
	NotFound         = &Error{Code: 10002, Message: "资源不存在", Status: 404}
	MethodNotAllowed = &Error{Code: 10003, Message: "请求方法不支持", Status: 405}
	TooLarge         = &Error{Code: 10004, Message: "请求内容过大", Status: 413}
	Unauthenticated  = &Error{Code: 20001, Message: "请先登录或重新登录", Status: 401}
	BadCredentials   = &Error{Code: 20002, Message: "用户名或密码错误", Status: 401}
	Forbidden        = &Error{Code: 20003, Message: "无权执行此操作", Status: 403}
	Conflict         = &Error{Code: 20004, Message: "用户名已被注册", Status: 409}
	UserNotFound     = &Error{Code: 20005, Message: "用户不存在", Status: 404}
	RoomGone         = &Error{Code: 30001, Message: "房间不存在或已关闭", Status: 404}
	RoomFull         = &Error{Code: 30002, Message: "房间已满或不存在", Status: 409}
	AlreadyInRoom    = &Error{Code: 30003, Message: "你已在房间中", Status: 409}
	NotInRoom        = &Error{Code: 30004, Message: "你不在对局中", Status: 409}
	NotYourTurn      = &Error{Code: 30005, Message: "还没轮到你", Status: 409}
	IllegalMove      = &Error{Code: 30006, Message: "不合法的走子", Status: 400}
	GameOver         = &Error{Code: 30007, Message: "对局已结束", Status: 409}
	BadPhase         = &Error{Code: 30008, Message: "当前阶段不能这样走", Status: 409}
	RateLimited      = &Error{Code: 30009, Message: "操作太频繁，请稍候", Status: 429}
	SelfJoin         = &Error{Code: 30010, Message: "不能加入自己创建的房间，请邀请另一位玩家", Status: 409}
	RoomNotReady     = &Error{Code: 30011, Message: "请等待好友加入并准备", Status: 409}
	RoomStarted      = &Error{Code: 30012, Message: "该房间已经开始游戏", Status: 409}
	Internal         = &Error{Code: 50000, Message: "服务器内部错误，请稍后重试", Status: 500}
	Unavailable      = &Error{Code: 50001, Message: "服务暂时不可用，请稍后重试", Status: 503}
)
