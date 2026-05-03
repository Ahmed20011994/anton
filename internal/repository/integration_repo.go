package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ahmed20011994/anton/internal/tenantctx"
)

type Integration struct {
	ID                   uuid.UUID
	TenantID             uuid.UUID
	SourceType           string
	CredentialsEncrypted []byte
	FieldMapping         json.RawMessage
	Enabled              bool
	LastSyncAt           *time.Time
	LastSyncStatus       *string
	LastSyncError        *string
}

type IntegrationRepo struct {
	pool *pgxpool.Pool
}

func NewIntegrationRepo(pool *pgxpool.Pool) *IntegrationRepo {
	return &IntegrationRepo{pool: pool}
}

const integrationCols = `
	id, tenant_id, source_type, credentials_encrypted, field_mapping, enabled,
	last_sync_at, last_sync_status, last_sync_error`

func scanIntegration(row pgx.Row) (Integration, error) {
	var i Integration
	err := row.Scan(
		&i.ID, &i.TenantID, &i.SourceType, &i.CredentialsEncrypted, &i.FieldMapping, &i.Enabled,
		&i.LastSyncAt, &i.LastSyncStatus, &i.LastSyncError,
	)
	return i, err
}

func (r *IntegrationRepo) Get(ctx context.Context, scope tenantctx.Scope, sourceType string) (Integration, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+integrationCols+`
		 FROM tenant_integrations
		 WHERE tenant_id = $1 AND source_type = $2`,
		scope.TenantID, sourceType,
	)
	i, err := scanIntegration(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Integration{}, ErrNotFound
	}
	if err != nil {
		return Integration{}, fmt.Errorf("Get: %w", err)
	}
	return i, nil
}

func (r *IntegrationRepo) Put(
	ctx context.Context,
	scope tenantctx.Scope,
	sourceType string,
	encrypted []byte,
	fieldMapping json.RawMessage,
) (Integration, error) {
	if len(fieldMapping) == 0 {
		fieldMapping = json.RawMessage(`{}`)
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO tenant_integrations (tenant_id, source_type, credentials_encrypted, field_mapping, enabled)
		VALUES ($1, $2, $3, $4, TRUE)
		ON CONFLICT (tenant_id, source_type) DO UPDATE SET
			credentials_encrypted = EXCLUDED.credentials_encrypted,
			field_mapping = EXCLUDED.field_mapping,
			updated_at = NOW()
		RETURNING `+integrationCols,
		scope.TenantID, sourceType, encrypted, fieldMapping,
	)
	i, err := scanIntegration(row)
	if err != nil {
		return Integration{}, fmt.Errorf("Put: %w", err)
	}
	return i, nil
}

// UpdateLastSyncSuccess advances the high-water mark and clears any
// previous error. Call this only after a successful sync.
func (r *IntegrationRepo) UpdateLastSyncSuccess(
	ctx context.Context,
	scope tenantctx.Scope,
	sourceType string,
) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tenant_integrations
		SET last_sync_at = NOW(),
			last_sync_status = 'ok',
			last_sync_error = NULL,
			updated_at = NOW()
		WHERE tenant_id = $1 AND source_type = $2`,
		scope.TenantID, sourceType,
	)
	if err != nil {
		return fmt.Errorf("UpdateLastSyncSuccess: %w", err)
	}
	return nil
}

// UpdateLastSyncFailure records that a sync failed without advancing
// last_sync_at. The next sync still considers everything since the previous
// successful run.
func (r *IntegrationRepo) UpdateLastSyncFailure(
	ctx context.Context,
	scope tenantctx.Scope,
	sourceType string,
	status string,
	errMsg string,
) error {
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE tenant_integrations
		SET last_sync_status = $3,
			last_sync_error = $4,
			updated_at = NOW()
		WHERE tenant_id = $1 AND source_type = $2`,
		scope.TenantID, sourceType, status, errPtr,
	)
	if err != nil {
		return fmt.Errorf("UpdateLastSyncFailure: %w", err)
	}
	return nil
}

func (r *IntegrationRepo) ListEnabledForTenant(ctx context.Context, tenantID uuid.UUID) ([]Integration, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+integrationCols+`
		 FROM tenant_integrations
		 WHERE tenant_id = $1 AND enabled = TRUE
		 ORDER BY source_type`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("ListEnabledForTenant: query: %w", err)
	}
	defer rows.Close()

	var out []Integration
	for rows.Next() {
		i, err := scanIntegration(rows)
		if err != nil {
			return nil, fmt.Errorf("ListEnabledForTenant: scan: %w", err)
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListEnabledForTenant: rows: %w", err)
	}
	return out, nil
}
