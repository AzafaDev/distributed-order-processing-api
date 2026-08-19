package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/AzafaDev/distributed-order-processing-api/internal/idempotency"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/httpx"
)

type idempotencyKey string

const idempotencyHeader idempotencyKey = "Idempotency-Key"

func IdempotencyMiddleware(s *idempotency.IdempotencyService, log *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, err := GetUserIDClaims(r.Context())
			if err != nil {
				log.ErrorContext(r.Context(), "idempotency middleware", "error", err)
				httpx.WriteErrorJSON(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			valueStr := r.Header.Get("Idempotency-Key")
			if valueStr == "" {
				httpx.WriteErrorJSON(w, http.StatusBadRequest, "idempotency-key header is required")
				return
			}

			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				log.ErrorContext(r.Context(), "idempotency middleware", "error", err)
				httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid request body payload")
				return
			}

			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

			hash := sha256.Sum256(bodyBytes)
			requestHash := hex.EncodeToString(hash[:])

			idempotencyResult, err := s.CheckAndClaim(r.Context(), valueStr, requestHash, userID)
			if err != nil {
				log.ErrorContext(r.Context(), "idempotency middleware", "error", err)
				httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
				return
			}

			switch idempotencyResult.Status {
			case idempotency.Replayed:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				w.Write(idempotencyResult.Response)
				return
			case idempotency.Mismatch:
				httpx.WriteErrorJSON(w, http.StatusConflict, "idempotency key already used with a different request body")
				return
			case idempotency.InProgress:
				httpx.WriteErrorJSON(w, http.StatusConflict, "a request with this idempotency key is still being processed")
				return
			}

			ctx := context.WithValue(r.Context(), idempotencyHeader, valueStr)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetIdempotencyValueFromContext(ctx context.Context) (string, error) {
	val, ok := ctx.Value(idempotencyHeader).(string)
	if !ok {
		return "", errors.New("failed to convert value of idempotencyHeader to string type")
	}
	return val, nil
}
