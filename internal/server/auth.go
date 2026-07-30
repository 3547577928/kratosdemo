package server

import (
	"context"
	"strings"

	"testdemo/pkg/auth"

	"github.com/go-kratos/kratos/v3/errors"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
)

// JWT Token 校验使用的密钥，需与 Login 生成时保持一致
const jwtSecret = "testdemo-jwt-secret-change-me-in-production"

// 不需要认证的白名单路径（完全匹配前缀）
var authWhitelist = []string{
	"/todo.v1.UserService/Login", // gRPC 操作名
	"/v1/users/login",            // HTTP 路径
}

// AuthMiddleware JWT 认证中间件
func AuthMiddleware() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			// 白名单跳过认证
			if op, ok := transport.FromServerContext(ctx); ok {
				path := op.Operation()
				for _, white := range authWhitelist {
					if strings.HasPrefix(path, white) || strings.Contains(path, white) {
						return handler(ctx, req)
					}
				}
			}

			// 从 Header 中提取 Authorization: Bearer <token>
			header := ""
			if tr, ok := transport.FromServerContext(ctx); ok {
				header = tr.RequestHeader().Get("Authorization")
			}
			if header == "" {
				return nil, errors.Unauthorized("UNAUTHORIZED", "missing Authorization header")
			}

			// 解析 Bearer 前缀
			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				return nil, errors.Unauthorized("UNAUTHORIZED", "invalid Authorization format, expect 'Bearer <token>'")
			}
			tokenStr := strings.TrimSpace(parts[1])
			if tokenStr == "" {
				return nil, errors.Unauthorized("UNAUTHORIZED", "empty token")
			}

			// 验证 Token
			claims, err := auth.ParseToken(jwtSecret, tokenStr)
			if err != nil {
				switch err {
				case auth.ErrTokenExpired:
					return nil, errors.Unauthorized("TOKEN_EXPIRED", "token expired")
				case auth.ErrTokenMissing:
					return nil, errors.Unauthorized("UNAUTHORIZED", "missing token")
				default:
					return nil, errors.Unauthorized("TOKEN_INVALID", "invalid token: "+err.Error())
				}
			}

			// 将解析出的用户信息存入 ctx，后续业务可使用
			ctx = context.WithValue(ctx, "auth_user_id", claims.UserID)
			ctx = context.WithValue(ctx, "auth_username", claims.Username)
			ctx = context.WithValue(ctx, "auth_role", claims.Role)

			return handler(ctx, req)
		}
	}
}
