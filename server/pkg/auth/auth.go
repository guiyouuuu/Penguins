// Package auth 用户认证：bcrypt 密码 + JWT 令牌。
package auth

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Claims JWT 载荷
type Claims struct {
	UserID   int64  `json:"uid"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Service 认证服务
type Service struct {
	secret []byte
}

// New 创建认证服务
func New(secret string) *Service {
	return &Service{secret: []byte(secret)}
}

// 常见错误
var (
	ErrBadCredentials = errors.New("用户名或密码错误")
	ErrInvalidToken   = errors.New("无效的登录凭证，请重新登录")
)

// ValidateUsername 用户名规则：3~16 字符，字母数字下划线或中文
func ValidateUsername(name string) error {
	n := utf8.RuneCountInString(name)
	if n < 2 || n > 16 {
		return errors.New("用户名需 2~16 个字符")
	}
	for _, r := range name {
		ok := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || (r >= 0x4e00 && r <= 0x9fa5)
		if !ok {
			return errors.New("用户名只能包含中文、字母、数字、下划线")
		}
	}
	return nil
}

// HashPassword 密码哈希
func HashPassword(password string) (string, error) {
	if len(password) < 6 || len(password) > 64 {
		return "", errors.New("密码需 6~64 位")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword 校验密码
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// IssueToken 签发 7 天有效的 JWT
func (s *Service) IssueToken(userID int64, username string) (string, error) {
	claims := Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "penguin-chess",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// ParseToken 解析并校验 JWT
func (s *Service) ParseToken(tokenStr string) (*Claims, error) {
	tokenStr = strings.TrimSpace(tokenStr)
	if tokenStr == "" {
		return nil, ErrInvalidToken
	}
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || claims.UserID <= 0 {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
