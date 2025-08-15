-- +goose Up
INSERT INTO products (id, name, description, price, stock, created_at, updated_at)
VALUES
  ('11111111-1111-1111-1111-111111111111','Starter Kit','Básico', 49.90, 10, NOW(), NOW()),
  ('22222222-2222-2222-2222-222222222222','Mouse Pro','Inalámbrico', 99.90,  5, NOW(), NOW()),
  ('33333333-3333-3333-3333-333333333333','Headset','Over-ear',   149.90, 7, NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DELETE FROM products WHERE id IN (
  '11111111-1111-1111-1111-111111111111',
  '22222222-2222-2222-2222-222222222222',
  '33333333-3333-3333-3333-333333333333'
);

