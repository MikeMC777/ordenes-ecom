// File: internal/product/repo.go
// Package product provides the repository interface and PostgreSQL implementation for managing products.
package product

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("product not found")
)

type Query struct {
	Q      string
	Limit  int
	Offset int
}

type Repository interface {
	Create(ctx context.Context, p *Product) error
	GetByID(ctx context.Context, id string) (*Product, error)
	List(ctx context.Context, q Query) ([]Product, error)
	Update(ctx context.Context, p *Product, updatePrice bool) error
	Delete(ctx context.Context, id string) (bool, error)
}

type PGRepo struct{ db *pgxpool.Pool }

func NewPGRepo(db *pgxpool.Pool) *PGRepo { return &PGRepo{db: db} }

func (r *PGRepo) Create(ctx context.Context, p *Product) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := r.db.Exec(ctx, `
		INSERT INTO products (id, name, description, price, stock, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,NOW(),NOW())
	`, p.ID, p.Name, p.Description, p.Price, p.Stock)
	return err
}

func (r *PGRepo) GetByID(ctx context.Context, id string) (*Product, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var p Product
	err := r.db.QueryRow(ctx, `
		SELECT id, name, description, price::text, stock, created_at, updated_at
		FROM products WHERE id=$1
	`, id).Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, ErrNotFound
	}
	return &p, nil
}

func (r *PGRepo) List(ctx context.Context, q Query) ([]Product, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	search := strings.TrimSpace(q.Q)

	rows, err := r.db.Query(ctx, `
		SELECT id, name, description, price::text, stock, created_at, updated_at
		FROM products
		WHERE ($1 = '' OR name ILIKE '%'||$1||'%' OR description ILIKE '%'||$1||'%')
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, search, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PGRepo) Update(ctx context.Context, p *Product, updatePrice bool) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if updatePrice {
		_, err := r.db.Exec(ctx, `
			UPDATE products
			SET name = COALESCE(NULLIF($2,''), name),
			    description = COALESCE(NULLIF($3,''), description),
			    price = $4,
			    stock = $5,
			    updated_at = NOW()
			WHERE id = $1
		`, p.ID, p.Name, p.Description, p.Price, p.Stock)
		return err
	}

	_, err := r.db.Exec(ctx, `
		UPDATE products
		SET name = COALESCE(NULLIF($2,''), name),
		    description = COALESCE(NULLIF($3,''), description),
		    stock = $4,
		    updated_at = NOW()
		WHERE id = $1
	`, p.ID, p.Name, p.Description, p.Stock)
	return err
}

func (r *PGRepo) Delete(ctx context.Context, id string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd, err := r.db.Exec(ctx, `DELETE FROM products WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}
