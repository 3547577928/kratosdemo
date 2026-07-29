package server

import (
	"context"

	"github.com/go-kratos/kratos/v3/log"
	"github.com/go-kratos/kratos/v3/middleware"
)

func Logmiddle() middleware.Middleware {
	logger := log.Default()
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			logger.Info("ok")
			return handler(ctx, req)
		}
	}
}
