-- name: LockProductForUpdate :one
SELECT * FROM products
WHERE id = $1
FOR UPDATE;

-- name: DecreaseProductStock :execrows
UPDATE products
SET stock = stock - $1,
    updated_at = now()
WHERE id = $2
    AND stock >= $1;

-- name: CreateOrder :one
INSERT INTO orders (
    user_id,
    total_amount
) VALUES ($1, $2)
RETURNING *;

-- name: CreateOrderItem :one
INSERT INTO order_items (
    order_id,
    product_id,
    quantity,
    price,
    subtotal
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreatePayment :one
INSERT INTO payments (
    order_id,
    amount,
    provider,
    transaction_id
) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetOrders :many
SELECT * FROM orders
WHERE user_id = $1;

-- name: GetOrderByID :one
SELECT * FROM orders
WHERE id = $1
    AND user_id = $2;

-- name: UpdateOrderStatus :one
UPDATE orders
SET status = $1,
    updated_at = now()
WHERE id = $2
    AND user_id = $3
RETURNING *;

-- name: GetOrderItemsByOrderID :many
SELECT * FROM order_items
WHERE order_id = $1;

-- name: IncreaseProductStock :execrows
UPDATE products
SET stock = stock + $1,
    updated_at = now()
WHERE id = $2;

-- name: LockOrderForUpdate :one
SELECT * FROM orders
WHERE user_id = $1
    AND id = $2
FOR UPDATE;