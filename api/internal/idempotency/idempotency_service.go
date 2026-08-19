package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const staleClaimTimeout = 30 * time.Second

var tracer = otel.Tracer("github.com/AzafaDev/distributed-order-processing-api/internal/idempotency")

type IdempotencyService struct {
	r Repository
}

func NewIdempotencyService(r Repository) *IdempotencyService {
	return &IdempotencyService{
		r: r,
	}
}

func (s *IdempotencyService) CheckAndClaim(ctx context.Context, key, requestHash string, userID uuid.UUID) (result *IdempotencyResult, err error) {
	ctx, span := tracer.Start(ctx, "idempotency.CheckAndClaim")
	defer func() {
		defer span.End()

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return
		}
		if result != nil {
			span.SetAttributes(attribute.String("idempotency.status", string(result.Status)))
		}
	}()

	_, err = s.r.CreateIdempotency(ctx, key, requestHash, userID)
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
