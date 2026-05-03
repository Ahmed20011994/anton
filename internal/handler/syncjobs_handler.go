package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/Ahmed20011994/anton/internal/httpx"
	"github.com/Ahmed20011994/anton/internal/repository"
	"github.com/Ahmed20011994/anton/internal/tenantctx"
)

type SyncJobsHandler struct {
	repo *repository.SyncJobRepo
}

func NewSyncJobsHandler(repo *repository.SyncJobRepo) *SyncJobsHandler {
	return &SyncJobsHandler{repo: repo}
}

// List handles GET /v1/tenants/{slug}/sync-jobs?status=&limit=&offset=.
func (h *SyncJobsHandler) List(w http.ResponseWriter, r *http.Request) {
	scope, ok := tenantctx.From(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "missing tenant scope")
		return
	}

	q := r.URL.Query()
	status := q.Get("status")
	limit := 50
	offset := 0
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}

	jobs, err := h.repo.List(r.Context(), scope, status, limit, offset)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"items": jobs,
		"count": len(jobs),
	})
}

// Get handles GET /v1/tenants/{slug}/sync-jobs/{job_id}.
func (h *SyncJobsHandler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := tenantctx.From(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "missing tenant scope")
		return
	}

	idStr := r.PathValue("job_id")
	jobID, err := uuid.Parse(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid job_id")
		return
	}

	job, err := h.repo.Get(r.Context(), scope, jobID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "sync job not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, job)
}
