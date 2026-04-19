package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ahmed20011994/anton/internal/handler"
)

func TestHealthHandler(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		wantStatus     int
		wantBodyStatus string
	}{
		{
			name:           "GET returns 200 with status ok",
			method:         http.MethodGet,
			wantStatus:     http.StatusOK,
			wantBodyStatus: "ok",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := handler.NewHealthHandler()
			req := httptest.NewRequest(tc.method, "/healthz", nil)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rec.Code, tc.wantStatus)
			}

			var body map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}

			if body["status"] != tc.wantBodyStatus {
				t.Errorf("body.status: got %q, want %q", body["status"], tc.wantBodyStatus)
			}

			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
			}
		})
	}
}
