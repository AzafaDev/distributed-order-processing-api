package idempotency

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

type IdempotencyService struct {
	r Repository
}

func NewIdempotencyService(r Repository) *IdempotencyService {
	return &IdempotencyService{
		r: r,
	}
}

func (s *IdempotencyService) CheckAndClaim(ctx context.Context, key, requestHash string, userID uuid.UUID) (*IdempotencyResult, error) {
	_, err := s.r.CreateIdempotency(ctx, key, requestHash, userID)
	if err == nil {
		return &IdempotencyResult{
			Status:   New,
			Response: nil,
		}, nil
	}

	if errors.Is(err, ErrIdempotencyKeyExists) {
		existingIdempotencyKey, err := s.r.GetIdempotencyByUserID(ctx, key, userID)
		if err != nil {
			return nil, err
		}

		if requestHash != existingIdempotencyKey.RequestHash {
			return &IdempotencyResult{
				Status:   Mismatch,
				Response: nil,
			}, nil
		}

		if len(existingIdempotencyKey.Response) == 0 {
			return &IdempotencyResult{
				Status:   InProgress,
				Response: nil,
			}, nil
		}

		return &IdempotencyResult{
			Status:   Replayed,
			Response: existingIdempotencyKey.Response,
		}, nil
	}

	return nil, err
}

func (s *IdempotencyService) SaveResponse(ctx context.Context, key string, userID uuid.UUID, response json.RawMessage) error {
	_, err := s.r.UpdateIdempotency(ctx, key, userID, response)
	if err != nil {
		return err
	}
	return nil
}
