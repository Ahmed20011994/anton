package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ahmed20011994/anton/internal/tenantctx"
)

const (
	JobStatusQueued     = "queued"
	JobStatusRunning    = "running"
	JobStatusCompleted  = "completed"
	JobStatusFailed     = "failed"
	JobStatusAuthFailed = "auth_failed"
)

type SyncJob struct {
	ID           uuid.UUID  `json:"id"`
	TenantID     uuid.UUID  `json:"tenant_id"`
	SourceType   string     `json:"source_type"`
	Status       string     `json:"status"`
	RequestedAt  time.Time  `json:"requested_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	HeartbeatAt  *time.Time `json:"heartbeat_at,omitempty"`
	WorkerID     *string    `json:"worker_id,omitempty"`
	SinceUsed    *time.Time `json:"since_used,omitempty"`
	CreatedCount int        `json:"created_count"`
	UpdatedCount int        `json:"updated_count"`
	FetchErrors  int        `json:"fetch_errors"`
	Error        *string    `json:"error,omitempty"`
}

type SyncJobRepo struct {
	pool *pgxpool.Pool
}

func NewSyncJobRepo(pool *pgxpool.Pool) *SyncJobRepo {
	return &SyncJobRepo{pool: pool}
}

const syncJobCols = `
	id, tenant_id, source_type, status, requested_at, started_at, finished_at,
	heartbeat_at, worker_id, since_used, created_count, updated_count,
	fetch_errors, error`

func scanSyncJob(row pgx.Row) (SyncJob, error) {
	var j SyncJob
	err := row.Scan(
		&j.ID, &j.TenantID, &j.SourceType, &j.Status, &j.RequestedAt,
		&j.StartedAt, &j.FinishedAt, &j.HeartbeatAt, &j.WorkerID,
		&j.SinceUsed, &j.CreatedCount, &j.UpdatedCount, &j.FetchErrors, &j.Error,
	)
	return j, err
}

// Enqueue inserts a queued job for (tenantID, sourceType). Multiple queued
// jobs for the same pair are allowed — they execute in FIFO order.
func (r *SyncJobRepo) Enqueue(ctx context.Context, tenantID uuid.UUID, sourceType string) (SyncJob, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO sync_jobs (tenant_id, source_type, status)
		VALUES ($1, $2, $3)
		RETURNING `+syncJobCols,
		tenantID, sourceType, JobStatusQueued,
	)
	j, err := scanSyncJob(row)
	if err != nil {
		return SyncJob{}, fmt.Errorf("Enqueue: %w", err)
	}
	return j, nil
}

// ClaimNext atomically picks the oldest queued job that has no concurrent
// running job for the same (tenant_id, source_type), flips its status to
// running, and returns it. (false, nil) means there's nothing to do.
func (r *SyncJobRepo) ClaimNext(ctx context.Context, workerID string) (SyncJob, bool, error) {
	row := r.pool.QueryRow(ctx, `
		WITH next AS (
			SELECT id FROM sync_jobs s
			WHERE s.status = 'queued'
			  AND NOT EXISTS (
			      SELECT 1 FROM sync_jobs r
			      WHERE r.tenant_id = s.tenant_id
			        AND r.source_type = s.source_type
			        AND r.status = 'running'
			  )
			ORDER BY s.requested_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE sync_jobs s
		SET status = 'running',
		    started_at = NOW(),
		    heartbeat_at = NOW(),
		    worker_id = $1
		FROM next
		WHERE s.id = next.id
		RETURNING s.id, s.tenant_id, s.source_type, s.status, s.requested_at,
		          s.started_at, s.finished_at, s.heartbeat_at, s.worker_id,
		          s.since_used, s.created_count, s.updated_count,
		          s.fetch_errors, s.error`,
		workerID,
	)
	j, err := scanSyncJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return SyncJob{}, false, nil
	}
	if err != nil {
		return SyncJob{}, false, fmt.Errorf("ClaimNext: %w", err)
	}
	return j, true, nil
}

// Heartbeat refreshes heartbeat_at on a running job. The worker_id check
// guards against another worker reclaiming the row mid-run.
func (r *SyncJobRepo) Heartbeat(ctx context.Context, jobID uuid.UUID, workerID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE sync_jobs
		SET heartbeat_at = NOW()
		WHERE id = $1 AND worker_id = $2 AND status = 'running'`,
		jobID, workerID,
	)
	if err != nil {
		return fmt.Errorf("Heartbeat: %w", err)
	}
	return nil
}

// UpdateStats writes incremental counts. Called every N items by the worker.
func (r *SyncJobRepo) UpdateStats(ctx context.Context, jobID uuid.UUID, created, updated, fetchErrors int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE sync_jobs
		SET created_count = $2,
		    updated_count = $3,
		    fetch_errors  = $4,
		    heartbeat_at  = NOW()
		WHERE id = $1`,
		jobID, created, updated, fetchErrors,
	)
	if err != nil {
		return fmt.Errorf("UpdateStats: %w", err)
	}
	return nil
}

// SetSinceUsed records the `since` value the worker passed to the connector.
// Useful for debugging "why did this sync pull so much".
func (r *SyncJobRepo) SetSinceUsed(ctx context.Context, jobID uuid.UUID, since time.Time) error {
	var sincePtr *time.Time
	if !since.IsZero() {
		sincePtr = &since
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE sync_jobs SET since_used = $2 WHERE id = $1`,
		jobID, sincePtr,
	)
	if err != nil {
		return fmt.Errorf("SetSinceUsed: %w", err)
	}
	return nil
}

// Complete marks the job as terminal. status must be one of the terminal
// constants (completed | failed | auth_failed). errMsg is empty for success.
func (r *SyncJobRepo) Complete(ctx context.Context, jobID uuid.UUID, status, errMsg string) error {
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE sync_jobs
		SET status = $2,
		    finished_at = NOW(),
		    error = $3
		WHERE id = $1`,
		jobID, status, errPtr,
	)
	if err != nil {
		return fmt.Errorf("Complete: %w", err)
	}
	return nil
}

// ReclaimStale flips running jobs whose heartbeat is older than maxAge back
// to queued. Run on server startup to recover jobs whose worker died.
// Returns the number of jobs reclaimed.
func (r *SyncJobRepo) ReclaimStale(ctx context.Context, maxAge time.Duration) (int, error) {
	cmd, err := r.pool.Exec(ctx, `
		UPDATE sync_jobs
		SET status = 'queued',
		    started_at = NULL,
		    heartbeat_at = NULL,
		    worker_id = NULL
		WHERE status = 'running'
		  AND (heartbeat_at IS NULL OR heartbeat_at < NOW() - $1::interval)`,
		fmt.Sprintf("%d milliseconds", maxAge.Milliseconds()),
	)
	if err != nil {
		return 0, fmt.Errorf("ReclaimStale: %w", err)
	}
	return int(cmd.RowsAffected()), nil
}

// List returns recent jobs for a tenant, optionally filtered by status.
func (r *SyncJobRepo) List(ctx context.Context, scope tenantctx.Scope, status string, limit, offset int) ([]SyncJob, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+syncJobCols+`
		FROM sync_jobs
		WHERE tenant_id = $1
		  AND ($2 = '' OR status = $2)
		ORDER BY requested_at DESC
		LIMIT $3 OFFSET $4`,
		scope.TenantID, status, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("List: query: %w", err)
	}
	defer rows.Close()

	out := make([]SyncJob, 0)
	for rows.Next() {
		j, err := scanSyncJob(rows)
		if err != nil {
			return nil, fmt.Errorf("List: scan: %w", err)
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("List: rows: %w", err)
	}
	return out, nil
}

// Get returns a single job within a tenant scope. Returns ErrNotFound when
// the job doesn't exist or belongs to a different tenant.
func (r *SyncJobRepo) Get(ctx context.Context, scope tenantctx.Scope, jobID uuid.UUID) (SyncJob, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+syncJobCols+`
		FROM sync_jobs
		WHERE id = $1 AND tenant_id = $2`,
		jobID, scope.TenantID,
	)
	j, err := scanSyncJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return SyncJob{}, ErrNotFound
	}
	if err != nil {
		return SyncJob{}, fmt.Errorf("Get: %w", err)
	}
	return j, nil
}
