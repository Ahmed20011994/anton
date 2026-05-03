package handler

import (
	"net/http"
	"strconv"

	"github.com/Ahmed20011994/anton/internal/httpx"
	"github.com/Ahmed20011994/anton/internal/repository"
	"github.com/Ahmed20011994/anton/internal/tenantctx"
)

type WorkItemHandler struct {
	repo *repository.WorkItemRepo
}

func NewWorkItemHandler(repo *repository.WorkItemRepo) *WorkItemHandler {
	return &WorkItemHandler{repo: repo}
}

// List handles GET /v1/tenants/{slug}/work-items.
// Query params: status_category, source_type, limit, offset.
func (h *WorkItemHandler) List(w http.ResponseWriter, r *http.Request) {
	scope, ok := tenantctx.From(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "missing tenant scope")
		return
	}

	q := r.URL.Query()
	f := repository.ListFilter{
		StatusCategory: q.Get("status_category"),
		SourceType:     q.Get("source_type"),
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Offset = n
		}
	}

	items, err := h.repo.List(r.Context(), scope, f)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}
