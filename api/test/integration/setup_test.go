//go:build integration

package integration

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/config"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/logger"
	"github.com/AzafaDev/distributed-order-processing-api/internal/server"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

type testEnv struct {
	baseURL string
	pool    *pgxpool.Pool
}

func setupTestEnv(t *testing.T) testEnv {
	t.Helper()

	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("orders_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, pgContainer.Terminate(context.Background()))
	})

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	redisContainer, err := tcredis.Run(ctx, "redis:8-alpine",
		tcredis.WithSnapshotting(10, 1),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, redisContainer.Terminate(context.Background()))
	})

	redisHost, err := redisContainer.Host(ctx)
	require.NoError(t, err)
	redisPort, err := redisContainer.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)

	runMigrations(t, dsn)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	cfg := &config.Config{
		DatabaseURL: dsn,
		JwtSecret:   "test-secret",
		JwtExpiry:   time.Hour,
		Port:        "0",
		GoEnv:       "test",
		RedisHost:   redisHost,
		RedisPort:   redisPort.Port(),
	}

	log := logger.New(cfg.GoEnv)

	app, err := server.BuildApp(ctx, cfg, log)
	require.NoError(t, err)
	t.Cleanup(func() {
		app.DB.Close()
		app.Redis.Close()
	})

	srv := httptest.NewServer(app.Router)
	t.Cleanup(srv.Close)

	return testEnv{baseURL: srv.URL, pool: pool}
}

func runMigrations(t *testing.T, dsn string) {
	t.Helper()

	m, err := migrate.New("file://../../migrations", dsn)
	require.NoError(t, err)

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		require.NoError(t, err)
	}
}
