package server

import (
	v1 "testdemo/api/todo/v1"
	"testdemo/internal/conf"
	"testdemo/internal/service"
	"time"

	aegisrate "github.com/go-kratos/aegis/ratelimit"
	"github.com/go-kratos/aegis/ratelimit/bbr"
	"github.com/go-kratos/kratos/contrib/otel/v3/metrics"
	"github.com/go-kratos/kratos/contrib/otel/v3/tracing"
	"github.com/go-kratos/kratos/v3/middleware/ratelimit"
	"github.com/go-kratos/kratos/v3/middleware/recovery"
	"github.com/go-kratos/kratos/v3/middleware/validate"
	"github.com/go-kratos/kratos/v3/transport/http"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.einride.tech/aip/fieldbehavior"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/protobuf/proto"
)

// NewHTTPServer new an HTTP serve r.
func NewHTTPServer(c *conf.Server, mp metric.MeterProvider, todo *service.TodoService, greeter *service.GreeterService, user *service.UserService) *http.Server {

	var opts = []http.ServerOption{
		http.Middleware(
			recovery.Recovery(),
			tracing.Server(),
			Logmiddle(),      //日志中间件
			AuthMiddleware(), //认证中间件
			ratelimit.Server(ratelimit.WithLimiter(NewRateLimiter())), //限流中间件
			metrics.Server(
				metrics.WithSeconds(metricSeconds),
				metrics.WithRequests(metricRequests),
			),
			validate.Validator(func(req any) error {
				if msg, ok := req.(proto.Message); ok {
					if err := fieldbehavior.ValidateRequiredFields(msg); err != nil {
						return err
					}
				}
				return nil
			}),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, http.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, http.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, http.Timeout(c.Http.Timeout.AsDuration()))
	}
	srv := http.NewServer(opts...)
	v1.RegisterTodoServiceHTTPServer(srv, todo)
	v1.RegisterGreeterHTTPServer(srv, greeter)
	v1.RegisterUserServiceHTTPServer(srv, user)
	srv.Route("/").GET("/metrics", func(ctx http.Context) error {
		promhttp.HandlerFor(metricGatherer, promhttp.HandlerOpts{}).ServeHTTP(ctx.Response(), ctx.Request())
		return nil
	})
	return srv
}

type bbrAdapter struct {
	limiter aegisrate.Limiter
}

// Allow 接口不兼容 ：Kratos v3 把 ratelimit 的核心接口移到了 internal/ratelimit ，导致 github.com/go-kratos/aegis/ratelimit 里的 Limiter 返回的 DoneFunc 类型
// 需要包装一下
func (b *bbrAdapter) Allow() (ratelimit.DoneFunc, error) {
	done, err := b.limiter.Allow()
	if err != nil {
		return nil, err
	}
	return func(info ratelimit.DoneInfo) {
		done(aegisrate.DoneInfo{Err: info.Err})
	}, nil
}
func NewRateLimiter() ratelimit.Limiter {
	return &bbrAdapter{
		limiter: bbr.NewLimiter(
			bbr.WithWindow(5*time.Second),
			bbr.WithBucket(100),
			bbr.WithCPUThreshold(80),
		),
	}
}
