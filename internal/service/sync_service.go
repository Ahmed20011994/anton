package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Ahmed20011994/anton/internal/canonical"
	"github.com/Ahmed20011994/anton/internal/connector"
	"github.com/Ahmed20011994/anton/internal/connector/jira"
	"github.com/Ahmed20011994/anton/internal/crypto"
	"github.com/Ahmed20011994/anton/internal/repository"
	"github.com/Ahmed20011994/anton/internal/tenantctx"
)

const (
	heartbeatInterval = 30 * time.Second
	statsBatchSize    = 50
)

type SyncService struct {
	integSvc     *IntegrationService
	integRepo    *repository.IntegrationRepo
	workItemRepo *repository.WorkItemRepo
	jobRepo      *repository.SyncJobRepo
	logger       *slog.Logger
}

func NewSyncService(
	integSvc *IntegrationService,
	integRepo *repository.IntegrationRepo,
	workItemRepo *repository.WorkItemRepo,
	jobRepo *repository.SyncJobRepo,
	logger *slog.Logger,
) *SyncService {
	if logger == nil {
		logger = slog.Default()
	}
	return &SyncService{
		integSvc:     integSvc,
		integRepo:    integRepo,
		workItemRepo: workItemRepo,
		jobRepo:      jobRepo,
		logger:       logger,
	}
}

// Enqueue creates a queued sync_jobs row. Workers pick it up later.
func (s *SyncService) Enqueue(ctx context.Context, scope tenantctx.Scope, sourceType string) (repository.SyncJob, error) {
	if sourceType == "" {
		return repository.SyncJob{}, fmt.Errorf("Enqueue: source_type required")
	}
	job, err := s.jobRepo.Enqueue(ctx, scope.TenantID, sourceType)
	if err != nil {
		return repository.SyncJob{}, fmt.Errorf("Enqueue: %w", err)
	}
	return job, nil
}

// RunJob is invoked by the worker pool for a claimed job. It loads
// credentials, runs the connector in streaming mode, upserts each item,
// keeps heartbeats alive, and finalizes the job row.
func (s *SyncService) RunJob(ctx context.Context, job repository.SyncJob, workerID string) {
	scope := tenantctx.Scope{TenantID: job.TenantID}

	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	go s.heartbeatLoop(hbCtx, job.ID, workerID)

	integ, pt, err := s.integSvc.LoadCredentials(ctx, scope, job.SourceType)
	defer crypto.Zero(pt)
	if err != nil {
		s.fail(ctx, job, scope, repository.JobStatusFailed, err)
		return
	}

	since := time.Time{}
	if integ.LastSyncAt != nil {
		since = integ.LastSyncAt.Add(-time.Hour)
	}
	_ = s.jobRepo.SetSinceUsed(ctx, job.ID, since)

	var (
		status      string
		runErr      error
		created     int
		updated     int
		fetchErrors int
	)

	switch job.SourceType {
	case "jira":
		status, runErr, created, updated, fetchErrors = s.runJira(ctx, scope, job, pt, since)
	default:
		s.fail(ctx, job, scope, repository.JobStatusFailed, fmt.Errorf("unknown source_type %q", job.SourceType))
		return
	}

	// Final stats write covers items since the last batch flush.
	if err := s.jobRepo.UpdateStats(ctx, job.ID, created, updated, fetchErrors); err != nil {
		s.logger.Error("update stats final", "job_id", job.ID, "err", err)
	}

	errMsg := ""
	if runErr != nil {
		errMsg = runErr.Error()
	}
	if err := s.jobRepo.Complete(ctx, job.ID, status, errMsg); err != nil {
		s.logger.Error("complete job", "job_id", job.ID, "err", err)
	}
	if status == repository.JobStatusCompleted {
		if err := s.integRepo.UpdateLastSyncSuccess(ctx, scope, job.SourceType); err != nil {
			s.logger.Error("update last sync success", "tenant", scope.TenantID, "source", job.SourceType, "err", err)
		}
	} else {
		if err := s.integRepo.UpdateLastSyncFailure(ctx, scope, job.SourceType, mapIntegrationStatus(status), errMsg); err != nil {
			s.logger.Error("update last sync failure", "tenant", scope.TenantID, "source", job.SourceType, "err", err)
		}
	}
}

func (s *SyncService) runJira(
	ctx context.Context,
	scope tenantctx.Scope,
	job repository.SyncJob,
	creds []byte,
	since time.Time,
) (status string, runErr error, created, updated, fetchErrors int) {
	var jc jira.Credentials
	if err := json.Unmarshal(creds, &jc); err != nil {
		return repository.JobStatusFailed, fmt.Errorf("decode jira creds: %w", err), 0, 0, 0
	}

	c := jira.New(jc)
	if err := c.Connect(ctx); err != nil {
		if errors.Is(err, connector.ErrAuth) {
			return repository.JobStatusAuthFailed, err, 0, 0, 0
		}
		return repository.JobStatusFailed, err, 0, 0, 0
	}

	batchCount := 0
	fetchErrs, syncErr := c.StreamSync(ctx, since, func(item canonical.WorkItem) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		item.TenantID = scope.TenantID
		outcome, err := s.workItemRepo.Upsert(ctx, scope, item)
		if err != nil {
			s.logger.Warn("upsert work item",
				"job_id", job.ID, "source_id", item.SourceID, "err", err)
			fetchErrors++
			return nil
		}
		switch outcome {
		case repository.OutcomeCreated:
			created++
		case repository.OutcomeUpdated:
			updated++
		}
		batchCount++
		if batchCount >= statsBatchSize {
			if err := s.jobRepo.UpdateStats(ctx, job.ID, created, updated, fetchErrors); err != nil {
				s.logger.Warn("update stats", "job_id", job.ID, "err", err)
			}
			batchCount = 0
		}
		return nil
	})

	fetchErrors += fetchErrs
	if syncErr != nil {
		return repository.JobStatusFailed, syncErr, created, updated, fetchErrors
	}
	return repository.JobStatusCompleted, nil, created, updated, fetchErrors
}

func (s *SyncService) heartbeatLoop(ctx context.Context, jobID uuid.UUID, workerID string) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.jobRepo.Heartbeat(ctx, jobID, workerID); err != nil {
				s.logger.Warn("heartbeat", "job_id", jobID, "err", err)
			}
		}
	}
}

// fail finalizes a job that errored before connector dispatch. It records
// the failure on the integration row but does NOT advance last_sync_at.
func (s *SyncService) fail(ctx context.Context, job repository.SyncJob, scope tenantctx.Scope, status string, err error) {
	msg := err.Error()
	if e := s.jobRepo.Complete(ctx, job.ID, status, msg); e != nil {
		s.logger.Error("fail: complete", "job_id", job.ID, "err", e)
	}
	if e := s.integRepo.UpdateLastSyncFailure(ctx, scope, job.SourceType, mapIntegrationStatus(status), msg); e != nil {
		s.logger.Error("fail: update last sync", "tenant", scope.TenantID, "err", e)
	}
}

func mapIntegrationStatus(jobStatus string) string {
	switch jobStatus {
	case repository.JobStatusCompleted:
		return "ok"
	case repository.JobStatusAuthFailed:
		return "auth_failed"
	default:
		return "error"
	}
}
