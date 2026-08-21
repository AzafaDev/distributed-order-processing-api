ALTER TABLE products
    DROP CONSTRAINT IF EXISTS products_price_check;

ALTER TABLE products
    ALTER COLUMN price TYPE NUMERIC(12, 2) USING price::NUMERIC(12, 2);
