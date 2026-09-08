// Package response 封装统一 HTTP 返回值。
package response

import (
	"encoding/json"
	"net/http"

	apperr "penguin-chess/server/pkg/errors"
	"penguin-chess/server/pkg/requestctx"
)

type Response struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data"`
	RequestID string `json:"requestId"`
}

func write(w http.ResponseWriter, r *http.Request, status int, value Response) {
	value.RequestID = requestctx.ID(r.Context())
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}

func Fail(w http.ResponseWriter, r *http.Request, err error) {
	e := apperr.Resolve(err)
	write(w, r, e.Status, Response{Code: e.Code, Message: e.Message, Data: nil})
}

func Success(w http.ResponseWriter, r *http.Request, data any) {
	write(w, r, http.StatusOK, Response{Code: 0, Message: "成功", Data: data})
}
