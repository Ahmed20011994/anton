package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Ahmed20011994/anton/internal/httpx"
	"github.com/Ahmed20011994/anton/internal/service"
	"github.com/Ahmed20011994/anton/internal/tenantctx"
)

type IntegrationHandler struct {
	svc *service.IntegrationService
}

func NewIntegrationHandler(svc *service.IntegrationService) *IntegrationHandler {
	return &IntegrationHandler{svc: svc}
}

// Put handles POST /v1/tenants/{slug}/integrations/{source}.
// The request body is the raw credentials JSON for the connector
// (e.g. {"email":"...","api_token":"...","base_url":"..."} for Jira).
func (h *IntegrationHandler) Put(w http.ResponseWriter, r *http.Request) {
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

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	if !json.Valid(body) {
		httpx.WriteError(w, http.StatusBadRequest, "credentials body must be valid JSON")
		return
	}

	integ, err := h.svc.StoreCredentials(r.Context(), scope, source, body, nil)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":          integ.ID,
		"tenant_id":   integ.TenantID,
		"source_type": integ.SourceType,
		"enabled":     integ.Enabled,
	})
}
