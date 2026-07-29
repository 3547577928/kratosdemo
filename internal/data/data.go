package data

import (
	"context"
	"fmt"
	"time"

	"testdemo/ent"
	"testdemo/internal/conf"

	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/go-kratos/kratos/v3/log"
	"github.com/google/wire"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewTodoRepo, NewUserRepo)

// Data wraps long-lived storage clients.
type Data struct {
	ent *ent.Client
}

// NewData opens an ent client backed by the configured database.
func NewData(c *conf.Data) (*Data, func(), error) {
	if c.Database.Driver != "postgres" && c.Database.Driver != "postgresql" {
		return nil, func() {}, fmt.Errorf("ent data layer expects postgres driver, got %q", c.Database.Driver)
	}

	client, err := ent.Open("postgres", c.Database.Source)
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed opening ent client: %w", err)
	}

	// Run migrations (optional; remove in favor of external migrations if preferred)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Schema.Create(ctx); err != nil {
		_ = client.Close()
		return nil, func() {}, fmt.Errorf("failed creating schema resources: %w", err)
	}

	cleanup := func() {
		log.Info("closing the data resources")
		_ = client.Close()
	}
	return &Data{ent: client}, cleanup, nil
}
