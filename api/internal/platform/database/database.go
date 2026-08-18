package database

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sqlc puts "-- name: CreateOrder :one" in first line every query.
var sqlcQueryName = regexp.MustCompile(`^--\s*name:\s*(\S+)`)

// spanNameFromSQL assigns a readable Jaeger span name.
// It extracts the query name from the sqlc comment,
// falling back to the first word for other statements (BEGIN, COMMIT, ping).
func spanNameFromSQL(stmt string) string {
	stmt = strings.TrimSpace(stmt)

	if m := sqlcQueryName.FindStringSubmatch(stmt); m != nil {
		return m[1]
	}

	if first, _, found := strings.Cut(stmt, " "); found {
		return first
	}

	return stmt
}

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
	cfgPool.ConnConfig.Tracer = otelpgx.NewTracer(
		// Required: otelpgx only invokes spanNameFunc when WithTrimSQLInSpanName is enabled.
		otelpgx.WithTrimSQLInSpanName(),
		otelpgx.WithSpanNameFunc(spanNameFromSQL),
	)

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
