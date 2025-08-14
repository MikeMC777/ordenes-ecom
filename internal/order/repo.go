package order

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("order not found")
)

type Repository interface {
	Create(ctx context.Context, o *Order, items []Item) error
	GetByID(ctx context.Context, id string) (*Order, []Item, error)
	ListByUser(ctx context.Context, userID string, limit, offset int) ([]Order, error)
	UpdateStatus(ctx context.Context, id, status string) error
	GetItems(ctx context.Context, orderID string) ([]Item, error)
}

type PGRepo struct{ db *pgxpool.Pool }

func NewPGRepo(db *pgxpool.Pool) *PGRepo { return &PGRepo{db: db} }

func (r *PGRepo) Create(ctx context.Context, o *Order, items []Item) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
    INSERT INTO orders (id, user_id, status, total, created_at, updated_at)
    VALUES ($1,$2,$3,$4,NOW(),NOW())
  `, o.ID, o.UserID, o.Status, o.Total); err != nil {
		return err
	}

	for _, it := range items {
		if _, err := tx.Exec(ctx, `
      INSERT INTO order_items (id, order_id, product_id, quantity, price)
      VALUES ($1,$2,$3,$4,$5)
    `, it.ID, o.ID, it.ProductID, it.Quantity, it.Price); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PGRepo) GetByID(ctx context.Context, id string) (*Order, []Item, error) {
	var o Order
	if err := r.db.QueryRow(ctx, `
    SELECT id,user_id,status,total::text,created_at,updated_at
    FROM orders WHERE id=$1
  `, id).Scan(&o.ID, &o.UserID, &o.Status, &o.Total, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return nil, nil, err
	}
	rows, err := r.db.Query(ctx, `
    SELECT id,order_id,product_id,quantity,price::text
    FROM order_items WHERE order_id=$1
  `, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.OrderID, &it.ProductID, &it.Quantity, &it.Price); err != nil {
			return nil, nil, err
		}
		items = append(items, it)
	}
	return &o, items, rows.Err()
}

func (r *PGRepo) ListByUser(ctx context.Context, userID string, limit, offset int) ([]Order, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.Query(ctx, `
    SELECT id,user_id,status,total::text,created_at,updated_at
    FROM orders WHERE user_id=$1
    ORDER BY created_at DESC LIMIT $2 OFFSET $3
  `, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.UserID, &o.Status, &o.Total, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *PGRepo) UpdateStatus(ctx context.Context, id, status string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tag, err := r.db.Exec(ctx, `
    UPDATE orders
    SET status = $2, updated_at = NOW()
    WHERE id = $1
  `, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PGRepo) GetItems(ctx context.Context, orderID string) ([]Item, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := r.db.Query(ctx, `
    SELECT id, order_id, product_id, quantity, price::text
    FROM order_items
    WHERE order_id = $1
  `, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.OrderID, &it.ProductID, &it.Quantity, &it.Price); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}
