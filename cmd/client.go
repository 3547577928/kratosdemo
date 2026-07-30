package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	v1 "testdemo/api/todo/v1"

	consul "github.com/go-kratos/kratos/contrib/registry/consul/v3"
	"github.com/go-kratos/kratos/v3/metadata"
	metadatamw "github.com/go-kratos/kratos/v3/middleware/metadata"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
	"github.com/hashicorp/consul/api"
)

func main() {
	var (
		consulAddr   string
		consulScheme string
		serviceName  string
		username     string
		password     string
		timeout      time.Duration
		pageSize     int
	)

	flag.StringVar(&consulAddr, "consul.addr", "127.0.0.1:8500", "consul address host:port")
	flag.StringVar(&consulScheme, "consul.scheme", "http", "consul scheme (http/https)")
	flag.StringVar(&serviceName, "service", "testdemo", "service name registered in consul")
	flag.StringVar(&username, "username", "", "login username (optional; required to call protected APIs)")
	flag.StringVar(&password, "password", "", "login password (optional; required to call protected APIs)")
	flag.DurationVar(&timeout, "timeout", 5*time.Second, "request timeout")
	flag.IntVar(&pageSize, "page_size", 10, "list page size")
	flag.Parse()

	cfg := api.DefaultConfig()
	cfg.Address = consulAddr
	cfg.Scheme = consulScheme

	consulClient, err := api.NewClient(cfg)
	if err != nil {
		log.Fatalf("create consul client failed: %v", err)
	}

	discovery := consul.New(consulClient)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	instances, err := discovery.GetService(ctx, serviceName)
	if err != nil {
		log.Fatalf("discovery.GetService failed: %v", err)
	}
	fmt.Printf("discovery ok: service=%s instances=%d\n", serviceName, len(instances))
	for _, ins := range instances {
		fmt.Printf("- id=%s name=%s version=%s endpoints=%v\n", ins.ID, ins.Name, ins.Version, ins.Endpoints)
	}

	httpClient, err := khttp.NewClient(
		context.Background(),
		khttp.WithDiscovery(discovery),
		khttp.WithEndpoint("discovery:///"+serviceName),
		khttp.WithTimeout(timeout),
		khttp.WithMiddleware(
			metadatamw.Client(),
		),
	)
	if err != nil {
		log.Fatalf("create http client failed: %v", err)
	}

	userClient := v1.NewUserServiceHTTPClient(httpClient)

	if username == "" || password == "" {
		_, err := userClient.ListUsers(context.Background(), &v1.ListUsersRequest{PageSize: int32(pageSize)})
		log.Fatalf("username/password not provided; ListUsers expected to fail with UNAUTHORIZED. got err=%v", err)
	}

	loginCtx, loginCancel := context.WithTimeout(context.Background(), timeout)
	defer loginCancel()

	loginReply, err := userClient.Login(loginCtx, &v1.LoginUserRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		log.Fatalf("Login failed: %v", err)
	}
	if loginReply.GetToken() == "" {
		log.Fatalf("Login returned empty token")
	}
	fmt.Printf("login ok, token_len=%d\n", len(loginReply.GetToken()))

	listCtx, listCancel := context.WithTimeout(context.Background(), timeout)
	defer listCancel()
	listCtx = metadata.AppendToClientContext(listCtx, "Authorization", "Bearer "+loginReply.GetToken())

	listReply, err := userClient.ListUsers(listCtx, &v1.ListUsersRequest{
		PageSize: int32(pageSize),
	})
	if err != nil {
		log.Fatalf("ListUsers failed: %v", err)
	}

	fmt.Printf("ListUsers ok, users=%d\n", len(listReply.GetUsers()))
	for _, u := range listReply.GetUsers() {
		fmt.Printf("id=%d name=%s email=%s\n", u.GetId(), u.GetName(), u.GetEmail())
	}
}
