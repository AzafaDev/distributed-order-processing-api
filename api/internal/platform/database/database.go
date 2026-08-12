package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*DB, error) {
	cfgPool, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	cfgPool.MaxConns = 10
	cfgPool.MinConns = 2
	cfgPool.MaxConnLifetime = time.Hour
	cfgPool.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfgPool)
	if err != nil {
		return nil, err
	}

	const (
		maxAttempts = 10
		retryDelay  = 2 * time.Second
	)

	var pingErr error

	for i := 1; i < maxAttempts; i++ {
		pingErr = pool.Ping(ctx)
		if pingErr == nil {
			return &DB{
				Pool: pool,
			}, nil
		}
		time.Sleep(retryDelay)
	}

	pool.Close()

	return nil, fmt.Errorf("db not ready after %d retries: %w", maxAttempts, pingErr)
}

func (d *DB) Ping(ctx context.Context) error {
	return d.Pool.Ping(ctx)
}

func (d *DB) Close() {
	d.Pool.Close()
}
