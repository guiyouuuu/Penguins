// Package request 定义 HTTP 请求参数及严格 JSON 解析。
package request

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	apperr "penguin-chess/server/pkg/errors"
)

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func DecodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return decodeError(err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return decodeError(err)
		}
		return apperr.InvalidArgument
	}
	return nil
}

func decodeError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return apperr.WithCause(apperr.TooLarge, err)
	}
	return apperr.WithCause(apperr.InvalidArgument, err)
}
