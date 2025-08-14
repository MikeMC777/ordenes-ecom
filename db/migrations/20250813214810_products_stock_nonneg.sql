-- +goose Up
ALTER TABLE products
  ADD CONSTRAINT products_stock_nonneg CHECK (stock >= 0);

-- +goose Down
ALTER TABLE products
  DROP CONSTRAINT IF EXISTS products_stock_nonneg;

