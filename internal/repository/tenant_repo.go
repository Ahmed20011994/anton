package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Tenant struct {
	ID         uuid.UUID
	Slug       string
	Name       string
	APIKeyHash string
}

type TenantRepo struct {
	pool *pgxpool.Pool
}

func NewTenantRepo(pool *pgxpool.Pool) *TenantRepo {
	return &TenantRepo{pool: pool}
}

func (r *TenantRepo) GetBySlug(ctx context.Context, slug string) (Tenant, error) {
	var t Tenant
	err := r.pool.QueryRow(ctx,
		`SELECT id, slug, name, api_key_hash FROM tenants WHERE slug = $1`,
		slug,
	).Scan(&t.ID, &t.Slug, &t.Name, &t.APIKeyHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, ErrNotFound
	}
	if err != nil {
		return Tenant{}, fmt.Errorf("GetBySlug: %w", err)
	}
	return t, nil
}

func (r *TenantRepo) Create(ctx context.Context, slug, name, apiKeyHash string) (Tenant, error) {
	var t Tenant
	err := r.pool.QueryRow(ctx, `
		INSERT INTO tenants (slug, name, api_key_hash)
		VALUES ($1, $2, $3)
		RETURNING id, slug, name, api_key_hash`,
		slug, name, apiKeyHash,
	).Scan(&t.ID, &t.Slug, &t.Name, &t.APIKeyHash)
	if err != nil {
		return Tenant{}, fmt.Errorf("Create: %w", err)
	}
	return t, nil
}

func (r *TenantRepo) ListAll(ctx context.Context) ([]Tenant, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, slug, name, api_key_hash FROM tenants ORDER BY slug`,
	)
	if err != nil {
		return nil, fmt.Errorf("ListAll: query: %w", err)
	}
	defer rows.Close()

	var out []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.APIKeyHash); err != nil {
			return nil, fmt.Errorf("ListAll: scan: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListAll: rows: %w", err)
	}
	return out, nil
}
