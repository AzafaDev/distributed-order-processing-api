package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/AzafaDev/distributed-order-processing-api/internal/idempotency/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNoIdempotencyFound   = errors.New("idempotency key not found")
	ErrIdempotencyKeyExists = errors.New("idempotency key exists")
)

type Repository interface {
	CreateIdempotency(ctx context.Context, key, requestHash string, userID uuid.UUID) (*IdempotencyKey, error)
	GetIdempotencyByUserID(ctx context.Context, key string, userID uuid.UUID) (*IdempotencyKey, error)
	UpdateIdempotency(ctx context.Context, key string, userID uuid.UUID, response json.RawMessage) (*IdempotencyKey, error)
	ReclaimStaleIdempotency(ctx context.Context, key, requestHash string, userID uuid.UUID, staleAfter time.Duration) (*IdempotencyKey, error)
}

type IdempotencyRepository struct {
	q *sqlc.Queries
}

func NewIdempotencyRepository(q *sqlc.Queries) Repository {
	return &IdempotencyRepository{
		q: q,
	}
}

func (r *IdempotencyRepository) CreateIdempotency(ctx context.Context, key, requestHash string, userID uuid.UUID) (*IdempotencyKey, error) {
	sqlcIdempotencyKey, err := r.q.CreateIdempotency(ctx, sqlc.CreateIdempotencyParams{
		Key:         key,
		RequestHash: requestHash,
		UserID: pgtype.UUID{
			Bytes: userID,
			Valid: true,
		},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrIdempotencyKeyExists
		}
		return nil, err
	}

	return &IdempotencyKey{
		Key:         sqlcIdempotencyKey.Key,
		UserID:      userID,
		RequestHash: requestHash,
		Response:    sqlcIdempotencyKey.Response,
		CreatedAt:   sqlcIdempotencyKey.CreatedAt.Time,
		UpdatedAt:   sqlcIdempotencyKey.UpdatedAt.Time,
	}, nil
}

func (r *IdempotencyRepository) GetIdempotencyByUserID(ctx context.Context, key string, userID uuid.UUID) (*IdempotencyKey, error) {
	sqlcIdempotencyKey, err := r.q.GetIdempotencyByUserID(ctx, sqlc.GetIdempotencyByUserIDParams{
		Key: key,
		UserID: pgtype.UUID{
			Bytes: userID,
			Valid: true,
		},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoIdempotencyFound
		}
		return nil, err
	}

	return &IdempotencyKey{
		Key:         sqlcIdempotencyKey.Key,
		UserID:      userID,
		RequestHash: sqlcIdempotencyKey.RequestHash,
		Response:    sqlcIdempotencyKey.Response,
		CreatedAt:   sqlcIdempotencyKey.CreatedAt.Time,
		UpdatedAt:   sqlcIdempotencyKey.UpdatedAt.Time,
	}, nil
}

func (r *IdempotencyRepository) ReclaimStaleIdempotency(ctx context.Context, key, requestHash string, userID uuid.UUID, staleAfter time.Duration) (*IdempotencyKey, error) {
	sqlcIdempotencyKey, err := r.q.ReclaimStaleIdempotency(ctx, sqlc.ReclaimStaleIdempotencyParams{
		RequestHash: requestHash,
		Key:         key,
		UserID: pgtype.UUID{
			Bytes: userID,
			Valid: true,
		},
		CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-staleAfter), Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoIdempotencyFound
		}
		return nil, err
	}

	return &IdempotencyKey{
		Key:         sqlcIdempotencyKey.Key,
		UserID:      userID,
		RequestHash: requestHash,
		Response:    sqlcIdempotencyKey.Response,
		CreatedAt:   sqlcIdempotencyKey.CreatedAt.Time,
		UpdatedAt:   sqlcIdempotencyKey.UpdatedAt.Time,
	}, nil
}

func (r *IdempotencyRepository) UpdateIdempotency(ctx context.Context, key string, userID uuid.UUID, response json.RawMessage) (*IdempotencyKey, error) {
	sqlcIdempotencyKey, err := r.q.UpdateIdempotency(ctx, sqlc.UpdateIdempotencyParams{
		Response: response,
		Key:      key,
		UserID: pgtype.UUID{
			Bytes: userID,
			Valid: true,
		},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoIdempotencyFound
		}
		return nil, err
	}

	return &IdempotencyKey{
		Key:         sqlcIdempotencyKey.Key,
		UserID:      userID,
		RequestHash: sqlcIdempotencyKey.RequestHash,
		Response:    sqlcIdempotencyKey.Response,
		CreatedAt:   sqlcIdempotencyKey.CreatedAt.Time,
		UpdatedAt:   sqlcIdempotencyKey.UpdatedAt.Time,
	}, nil
}
