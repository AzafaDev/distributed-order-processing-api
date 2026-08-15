package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

const staleClaimTimeout = 30 * time.Second

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
			_, reclaimErr := s.r.ReclaimStaleIdempotency(ctx, key, requestHash, userID, staleClaimTimeout)
			if reclaimErr != nil {
				if errors.Is(reclaimErr, ErrNoIdempotencyFound) {
					return &IdempotencyResult{
						Status:   InProgress,
						Response: nil,
					}, nil
				}
				return nil, reclaimErr
			}

			return &IdempotencyResult{
				Status:   New,
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
