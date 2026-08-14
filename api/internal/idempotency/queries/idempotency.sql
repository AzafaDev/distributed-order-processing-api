-- name: GetIdempotencyByUserID :one
SELECT * FROM idempotency_keys
WHERE key = $1
    AND user_id = $2;

-- name: CreateIdempotency :one
INSERT INTO idempotency_keys (
    key,
    user_id,
    request_hash
) VALUES ($1, $2, $3)
ON CONFLICT (key) DO NOTHING
RETURNING *;

-- name: UpdateIdempotency :one
UPDATE idempotency_keys
SET response = $1,
    updated_at = now()
WHERE key = $2
    AND user_id = $3
RETURNING *;