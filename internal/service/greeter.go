package service

import (
	"context"
	"fmt"

	v1 "testdemo/api/todo/v1"
)

// GreeterService is a greeter service.
type GreeterService struct {
	v1.UnimplementedGreeterServer
}

// NewGreeterService new a greeter service.
func NewGreeterService() *GreeterService {
	return &GreeterService{}
}

// SayHello sends a greeting.
func (s *GreeterService) SayHello(ctx context.Context, req *v1.HelloRequest) (*v1.HelloReply, error) {
	return &v1.HelloReply{
		Message: fmt.Sprintf("Hello %s", req.GetName()),
	}, nil
}
