package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ahmed20011994/anton/internal/canonical"
	"github.com/Ahmed20011994/anton/internal/tenantctx"
)

type UpsertOutcome int

const (
	OutcomeCreated UpsertOutcome = iota
	OutcomeUpdated
)

type WorkItemRepo struct {
	pool *pgxpool.Pool
}

func NewWorkItemRepo(pool *pgxpool.Pool) *WorkItemRepo {
	return &WorkItemRepo{pool: pool}
}

func (r *WorkItemRepo) Upsert(ctx context.Context, scope tenantctx.Scope, item canonical.WorkItem) (UpsertOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return OutcomeCreated, fmt.Errorf("Upsert: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existing string
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(content_hash, '') FROM work_items
		WHERE tenant_id = $1 AND source_type = $2 AND source_id = $3`,
		scope.TenantID, item.SourceType, item.SourceID,
	).Scan(&existing)

	outcome := OutcomeCreated
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		outcome = OutcomeCreated
	case err != nil:
		return OutcomeCreated, fmt.Errorf("Upsert: probe: %w", err)
	default:
		outcome = OutcomeUpdated
	}

	assignees := item.Assignees
	if assignees == nil {
		assignees = []string{}
	}
	linked := item.LinkedCustomerSignals
	if linked == nil {
		linked = []string{}
	}
	comments := item.Comments
	if len(comments) == 0 {
		comments = json.RawMessage(`[]`)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO work_items (
			tenant_id, source_id, source_type, item_type, title, description,
			status, status_category, priority, assignees,
			created_at, updated_at, closed_at, reopen_count, comments,
			linked_customer_signals, version, raw_payload, content_hash
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13, $14, $15,
			$16, $17, $18, $19
		)
		ON CONFLICT (tenant_id, source_type, source_id) DO UPDATE SET
			item_type = EXCLUDED.item_type,
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			status = EXCLUDED.status,
			status_category = EXCLUDED.status_category,
			priority = EXCLUDED.priority,
			assignees = EXCLUDED.assignees,
			updated_at = EXCLUDED.updated_at,
			closed_at = EXCLUDED.closed_at,
			reopen_count = EXCLUDED.reopen_count,
			comments = EXCLUDED.comments,
			linked_customer_signals = EXCLUDED.linked_customer_signals,
			version = EXCLUDED.version,
			raw_payload = EXCLUDED.raw_payload,
			content_hash = EXCLUDED.content_hash`,
		scope.TenantID, item.SourceID, item.SourceType,
		nullableString(item.ItemType),
		nullableString(item.Title),
		nullableString(item.Description),
		nullableString(item.Status),
		nullableString(item.StatusCategory),
		nullableString(item.Priority),
		assignees,
		nullableTime(item.CreatedAt),
		nullableTime(item.UpdatedAt),
		item.ClosedAt,
		item.ReopenCount,
		comments,
		linked,
		item.Version,
		nullableJSON(item.RawPayload),
		nullableString(item.ContentHash),
	)
	if err != nil {
		return OutcomeCreated, fmt.Errorf("Upsert: exec: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return OutcomeCreated, fmt.Errorf("Upsert: commit: %w", err)
	}
	return outcome, nil
}

type ListFilter struct {
	StatusCategory string
	SourceType     string
	Limit          int
	Offset         int
}

func (r *WorkItemRepo) List(ctx context.Context, scope tenantctx.Scope, f ListFilter) ([]canonical.WorkItem, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT tenant_id, source_id, source_type,
		       COALESCE(item_type,''), COALESCE(title,''), COALESCE(description,''),
		       COALESCE(status,''), COALESCE(status_category,''), COALESCE(priority,''),
		       assignees,
		       COALESCE(created_at, '1970-01-01'::timestamptz),
		       COALESCE(updated_at, '1970-01-01'::timestamptz),
		       closed_at, reopen_count,
		       COALESCE(comments, '[]'::jsonb),
		       linked_customer_signals, version,
		       raw_payload, COALESCE(content_hash,'')
		FROM work_items
		WHERE tenant_id = $1
		  AND ($2 = '' OR status_category = $2)
		  AND ($3 = '' OR source_type = $3)
		ORDER BY updated_at DESC
		LIMIT $4 OFFSET $5`,
		scope.TenantID, f.StatusCategory, f.SourceType, f.Limit, f.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("List: query: %w", err)
	}
	defer rows.Close()

	out := make([]canonical.WorkItem, 0)
	for rows.Next() {
		var w canonical.WorkItem
		if err := rows.Scan(
			&w.TenantID, &w.SourceID, &w.SourceType,
			&w.ItemType, &w.Title, &w.Description,
			&w.Status, &w.StatusCategory, &w.Priority,
			&w.Assignees,
			&w.CreatedAt, &w.UpdatedAt,
			&w.ClosedAt, &w.ReopenCount,
			&w.Comments,
			&w.LinkedCustomerSignals, &w.Version,
			&w.RawPayload, &w.ContentHash,
		); err != nil {
			return nil, fmt.Errorf("List: scan: %w", err)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("List: rows: %w", err)
	}
	return out, nil
}
