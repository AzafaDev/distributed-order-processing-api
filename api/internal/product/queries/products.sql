-- name: ListProducts :many
SELECT *
FROM products
LIMIT $1 OFFSET $2;
-- name: GetProductByID :one
SELECT *
FROM products
WHERE id = $1;
-- name: CreateProduct :one
INSERT INTO products (
    name,
    description,
    price,
    stock
) VALUES ($1, $2, $3, $4)
RETURNING *;
-- name: UpdateProduct :one
UPDATE products
SET name = COALESCE(sqlc.narg(name), name),
    description = COALESCE(sqlc.narg(description), description),
    price = COALESCE(sqlc.narg(price), price),
    stock = COALESCE(sqlc.narg(stock), stock),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;
-- name: DeleteProductByID :execrows
DELETE FROM products
WHERE id = $1;
-- name: GetProductByName :one
SELECT * FROM products
WHERE name = $1;