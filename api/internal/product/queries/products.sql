-- name: ListProducts :many
SELECT *
FROM products
LIMIT $1 OFFSET $2;
-- name: GetProductByID :one
SELECT *
FROM products
WHERE id = $1;