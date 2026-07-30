package server

import (
	"github.com/google/wire"
)

// ProviderSet is server providers.//添加要重新make all
var ProviderSet = wire.NewSet(NewGRPCServer, NewHTTPServer, NewMeterProvider)
