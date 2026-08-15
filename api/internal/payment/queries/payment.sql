-- name: LockPaymentForUpdate :one
SELECT * FROM payments
WHERE order_id = $1
FOR UPDATE;

-- name: UpdatePaymentStatus :one
UPDATE payments
SET status = $1,
    transaction_id = $2,
    updated_at = now()
WHERE order_id = $3
RETURNING *;

-- name: GetPaymentByOrderID :one
SELECT p.* FROM payments p
JOIN orders o ON o.id = p.order_id
WHERE o.id = $1
    AND o.user_id = $2;

-- name: LockOrderForPayment :one
SELECT * FROM orders
WHERE id = $1
    AND user_id = $2
FOR UPDATE;

-- name: MarkOrderAsPaid :one
UPDATE orders
SET status = 'paid',
    updated_at = now()
WHERE id = $1
RETURNING *;