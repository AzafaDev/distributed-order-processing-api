-- Prices are integer minor units everywhere else in the schema
-- (orders.total_amount, order_items.price are both BIGINT). NUMERIC(12,2) on
-- products was the odd one out: it advertises a fractional part that the write
-- path never produces and the order path silently rounds away.
ALTER TABLE products
    ALTER COLUMN price TYPE BIGINT USING round(price)::BIGINT;

ALTER TABLE products
    ADD CONSTRAINT products_price_check CHECK (price >= 0);
