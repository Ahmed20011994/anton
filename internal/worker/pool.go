package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Ahmed20011994/anton/internal/repository"
	"github.com/Ahmed20011994/anton/internal/service"
)

const (
	idleSleep      = 2 * time.Second
	claimErrorWait = 5 * time.Second
)

type Pool struct {
	repo     *repository.SyncJobRepo
	runner   *service.SyncService
	workerN  int
	workerID string
	logger   *slog.Logger
	wg       sync.WaitGroup
}

func NewPool(
	repo *repository.SyncJobRepo,
	runner *service.SyncService,
	workerN int,
	workerID string,
	logger *slog.Logger,
) *Pool {
	if workerN <= 0 {
		workerN = 1
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Pool{
		repo:     repo,
		runner:   runner,
		workerN:  workerN,
		workerID: workerID,
		logger:   logger,
	}
}

// Start spawns workerN goroutines. Cancel ctx to stop them; call Wait
// afterwards to block until in-flight jobs finish.
func (p *Pool) Start(ctx context.Context) {
	p.logger.Info("worker pool started", "workers", p.workerN, "worker_id", p.workerID)
	for i := 0; i < p.workerN; i++ {
		id := fmt.Sprintf("%s-%d", p.workerID, i)
		p.wg.Add(1)
		go p.loop(ctx, id)
	}
}

// Wait blocks until all workers have exited.
func (p *Pool) Wait() {
	p.wg.Wait()
}

func (p *Pool) loop(ctx context.Context, id string) {
	defer p.wg.Done()
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		job, ok, err := p.repo.ClaimNext(ctx, id)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			p.logger.Error("worker claim", "worker", id, "err", err)
			if !sleep(ctx, claimErrorWait) {
				return
			}
			continue
		}
		if !ok {
			if !sleep(ctx, idleSleep) {
				return
			}
			continue
		}
		p.logger.Info("worker claimed", "worker", id, "job_id", job.ID, "tenant_id", job.TenantID, "source", job.SourceType)
		p.runner.RunJob(ctx, job, id)
	}
}

// sleep waits for d or until ctx is done. Returns false if ctx was done.
func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
