package server

import (
	"testdemo/internal/conf"

	consul "github.com/go-kratos/kratos/contrib/registry/consul/v3"
	"github.com/go-kratos/kratos/v3/registry"
	"github.com/google/wire"
	"github.com/hashicorp/consul/api"
)

func NewRegistry(c *conf.Registry) *consul.Registry {
	if c == nil || c.Consul == nil {
		return nil
	}
	cfg := api.DefaultConfig()
	cfg.Address = c.Consul.Address
	cfg.Scheme = c.Consul.Scheme
	cli, err := api.NewClient(cfg)
	if err != nil {
		panic(err)
	}
	return consul.New(cli)
}

func NewRegistrar(r *consul.Registry) registry.Registrar { return r }
func NewDiscovery(r *consul.Registry) registry.Discovery { return r }

// ProviderSet is server providers.//添加要重新make all
var ProviderSet = wire.NewSet(NewGRPCServer, NewHTTPServer, NewMeterProvider, NewRegistry, NewRegistrar, NewDiscovery)
