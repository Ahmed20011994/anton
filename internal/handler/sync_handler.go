package handler

import (
	"net/http"

	"github.com/Ahmed20011994/anton/internal/httpx"
	"github.com/Ahmed20011994/anton/internal/service"
	"github.com/Ahmed20011994/anton/internal/tenantctx"
)

type SyncHandler struct {
	svc *service.SyncService
}

func NewSyncHandler(svc *service.SyncService) *SyncHandler {
	return &SyncHandler{svc: svc}
}

// Enqueue handles POST /v1/tenants/{slug}/sync/{source}.
// Returns 202 Accepted with the queued sync_jobs row. Workers pick it up
// asynchronously; clients poll GET /sync-jobs/{id} for status.
func (h *SyncHandler) Enqueue(w http.ResponseWriter, r *http.Request) {
	scope, ok := tenantctx.From(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "missing tenant scope")
		return
	}
	source := r.PathValue("source")
	if source == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing source type")
		return
	}

	job, err := h.svc.Enqueue(r.Context(), scope, source)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, job)
}
