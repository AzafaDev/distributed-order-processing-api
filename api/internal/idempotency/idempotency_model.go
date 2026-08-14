package idempotency

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type IdempotencyKey struct {
	Key         string    `json:"key"`
	UserID      uuid.UUID `json:"user_id"`
	RequestHash string    `json:"request_hash"`
	Response    []byte    `json:"response"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type IdempotencyKeyStatus string

type IdempotencyResult struct {
	Status   IdempotencyKeyStatus `json:"status"`
	Response json.RawMessage      `json:"response"`
}

const (
	New        IdempotencyKeyStatus = "new"
	Replayed   IdempotencyKeyStatus = "replayed"
	Mismatch   IdempotencyKeyStatus = "mismatch"
	InProgress IdempotencyKeyStatus = "in_progress"
)
