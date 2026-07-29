package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// 常见鉴权错误，供上层 service / middleware 做类型判断
var (
	ErrTokenMissing   = errors.New("auth: token missing")
	ErrTokenInvalid   = errors.New("auth: token invalid")
	ErrTokenExpired   = errors.New("auth: token expired")
	ErrRoleForbidden  = errors.New("auth: role forbidden")
	ErrMethodMismatch = errors.New("auth: signing method mismatch")
)

// CustomClaims 自定义载荷结构体
type CustomClaims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken 颁发 JWT 令牌
// secret: 密钥；expireHours: 过期时间（小时）
func GenerateToken(secret string, userID int64, username string, role string, expireHours int) (string, error) {
	claims := CustomClaims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expireHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "your-service-name",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

// ParseToken 校验并解析 JWT 字符串，返回自定义 Claims
// secret: 与 GenerateToken 一致的共享密钥
// allowedIssuer: 可选的签发人白名单；空字符串表示不校验签发人
func ParseToken(secret string, tokenString string, allowedIssuer ...string) (*CustomClaims, error) {
	if tokenString == "" {
		return nil, ErrTokenMissing
	}
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(t *jwt.Token) (any, error) {
		// 防止「算法切换攻击」：必须和签发时保持一致（HS256）
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrMethodMismatch, t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, ErrTokenExpired
		case errors.Is(err, jwt.ErrTokenNotValidYet):
			return nil, ErrTokenInvalid
		default:
			return nil, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
		}
	}
	claims, ok := token.Claims.(*CustomClaims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}
	// 可选签发人校验
	if len(allowedIssuer) > 0 && allowedIssuer[0] != "" {
		if claims.Issuer != allowedIssuer[0] {
			return nil, ErrTokenInvalid
		}
	}
	return claims, nil
}

// HasRole 判断当前 claims 是否包含任一指定角色（不区分大小写）
func (c *CustomClaims) HasRole(roles ...string) bool {
	if c == nil {
		return false
	}
	for _, r := range roles {
		if c.Role == r {
			return true
		}
	}
	return false
}
