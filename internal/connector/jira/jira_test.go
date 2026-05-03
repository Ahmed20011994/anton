package jira_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ahmed20011994/anton/internal/canonical"
	"github.com/Ahmed20011994/anton/internal/connector"
	"github.com/Ahmed20011994/anton/internal/connector/jira"
)

func TestSync_FieldMapping(t *testing.T) {
	tests := []struct {
		name          string
		issueJSON     string
		wantSourceID  string
		wantItemType  string
		wantStatusCat string
		wantPriority  string
		wantAssignees []string
		wantLinks     []string
		wantTitle     string
		wantHasClosed bool
		wantVersion   string
	}{
		{
			name: "bug in progress",
			issueJSON: `{
				"id":"10001","key":"PROJ-1",
				"fields":{
					"summary":"Mobile app crashes",
					"description":"crash on checkout",
					"status":{"name":"In Progress","statusCategory":{"key":"indeterminate"}},
					"issuetype":{"name":"Bug"},
					"priority":{"name":"High"},
					"assignee":{"displayName":"Alice","emailAddress":"a@x.com"},
					"created":"2026-04-01T10:00:00.000+0000",
					"updated":"2026-04-15T12:00:00.000+0000",
					"issuelinks":[],
					"comment":{"comments":[]}
				}
			}`,
			wantSourceID:  "PROJ-1",
			wantItemType:  "bug",
			wantStatusCat: "in_progress",
			wantPriority:  "high",
			wantAssignees: []string{"Alice"},
			wantTitle:     "Mobile app crashes",
		},
		{
			name: "story done with fix version",
			issueJSON: `{
				"id":"10002","key":"PROJ-2",
				"fields":{
					"summary":"SSO login",
					"status":{"name":"Done","statusCategory":{"key":"done"}},
					"issuetype":{"name":"Story"},
					"resolutiondate":"2026-04-10T08:00:00.000+0000",
					"fixVersions":[{"name":"v1.2"}]
				}
			}`,
			wantSourceID:  "PROJ-2",
			wantItemType:  "feature",
			wantStatusCat: "done",
			wantTitle:     "SSO login",
			wantHasClosed: true,
			wantVersion:   "v1.2",
		},
		{
			name: "task with customer link",
			issueJSON: `{
				"id":"10003","key":"PROJ-3",
				"fields":{
					"summary":"Investigate timeout",
					"status":{"statusCategory":{"key":"new"}},
					"issuetype":{"name":"Task"},
					"issuelinks":[
						{"type":{"name":"Customer Issue"},"inwardIssue":{"key":"SUP-99"}},
						{"type":{"name":"Blocks"},"outwardIssue":{"key":"PROJ-77"}}
					]
				}
			}`,
			wantSourceID:  "PROJ-3",
			wantItemType:  "task",
			wantStatusCat: "todo",
			wantLinks:     []string{"SUP-99"},
			wantTitle:     "Investigate timeout",
		},
		{
			name: "unknown issuetype falls back to task",
			issueJSON: `{
				"id":"10004","key":"PROJ-4",
				"fields":{
					"summary":"Misc",
					"issuetype":{"name":"Spike"},
					"status":{"statusCategory":{"key":"new"}}
				}
			}`,
			wantSourceID: "PROJ-4",
			wantItemType: "task",
			wantTitle:    "Misc",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newSinglePageServer(tc.issueJSON)
			defer srv.Close()

			c := jira.New(jira.Credentials{
				Email: "x@y.com", APIToken: "t", BaseURL: srv.URL,
			}).WithHTTPClient(srv.Client())

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			items := collect(t, c, ctx, time.Time{})
			if len(items) != 1 {
				t.Fatalf("items: got %d, want 1", len(items))
			}
			w := items[0]
			if w.SourceID != tc.wantSourceID {
				t.Errorf("SourceID: got %q, want %q", w.SourceID, tc.wantSourceID)
			}
			if w.SourceType != "jira" {
				t.Errorf("SourceType: got %q, want %q", w.SourceType, "jira")
			}
			if w.ItemType != tc.wantItemType {
				t.Errorf("ItemType: got %q, want %q", w.ItemType, tc.wantItemType)
			}
			if w.StatusCategory != tc.wantStatusCat {
				t.Errorf("StatusCategory: got %q, want %q", w.StatusCategory, tc.wantStatusCat)
			}
			if w.Title != tc.wantTitle {
				t.Errorf("Title: got %q, want %q", w.Title, tc.wantTitle)
			}
			if tc.wantPriority != "" && w.Priority != tc.wantPriority {
				t.Errorf("Priority: got %q, want %q", w.Priority, tc.wantPriority)
			}
			if tc.wantAssignees != nil && !equalStrings(w.Assignees, tc.wantAssignees) {
				t.Errorf("Assignees: got %v, want %v", w.Assignees, tc.wantAssignees)
			}
			if tc.wantLinks != nil && !equalStrings(w.LinkedCustomerSignals, tc.wantLinks) {
				t.Errorf("LinkedCustomerSignals: got %v, want %v", w.LinkedCustomerSignals, tc.wantLinks)
			}
			if tc.wantHasClosed && w.ClosedAt == nil {
				t.Errorf("ClosedAt: got nil, want set")
			}
			if !tc.wantHasClosed && w.ClosedAt != nil {
				t.Errorf("ClosedAt: got %v, want nil", w.ClosedAt)
			}
			if tc.wantVersion != "" && (w.Version == nil || *w.Version != tc.wantVersion) {
				t.Errorf("Version: got %v, want %q", w.Version, tc.wantVersion)
			}
			if w.ContentHash == "" {
				t.Errorf("ContentHash: empty")
			}
			if len(w.RawPayload) == 0 {
				t.Errorf("RawPayload: empty")
			}
		})
	}
}

func TestSync_Pagination(t *testing.T) {
	tests := []struct {
		name      string
		pages     []string
		wantItems int
	}{
		{
			name:      "single page",
			pages:     []string{`{"issues":[{"id":"1","key":"P-1","fields":{"summary":"a"}}],"isLast":true}`},
			wantItems: 1,
		},
		{
			name: "two pages",
			pages: []string{
				`{"issues":[{"id":"1","key":"P-1","fields":{"summary":"a"}}],"nextPageToken":"abc","isLast":false}`,
				`{"issues":[{"id":"2","key":"P-2","fields":{"summary":"b"}}],"isLast":true}`,
			},
			wantItems: 2,
		},
		{
			name:      "empty result",
			pages:     []string{`{"issues":[],"isLast":true}`},
			wantItems: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/search/jql") {
					http.Error(w, "wrong path", http.StatusNotFound)
					return
				}
				if calls >= len(tc.pages) {
					http.Error(w, "extra call", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.pages[calls]))
				calls++
			}))
			defer srv.Close()

			c := jira.New(jira.Credentials{
				Email: "x", APIToken: "y", BaseURL: srv.URL,
			}).WithHTTPClient(srv.Client())

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			items := collect(t, c, ctx, time.Time{})
			if len(items) != tc.wantItems {
				t.Errorf("items: got %d, want %d", len(items), tc.wantItems)
			}
			if calls != len(tc.pages) {
				t.Errorf("calls: got %d, want %d", calls, len(tc.pages))
			}
		})
	}
}

func TestConnect_AuthErrors(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		wantAuthErr bool
	}{
		{name: "401", status: http.StatusUnauthorized, wantAuthErr: true},
		{name: "403", status: http.StatusForbidden, wantAuthErr: true},
		{name: "200", status: http.StatusOK, wantAuthErr: false},
		{name: "500", status: http.StatusInternalServerError, wantAuthErr: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			c := jira.New(jira.Credentials{
				Email: "x", APIToken: "y", BaseURL: srv.URL,
			}).WithHTTPClient(srv.Client())

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := c.Connect(ctx)
			gotAuth := err != nil && errors.Is(err, connector.ErrAuth)
			if gotAuth != tc.wantAuthErr {
				t.Errorf("auth err: got %v (err=%v), want %v", gotAuth, err, tc.wantAuthErr)
			}
		})
	}
}

func TestBuildJQL_Smoke(t *testing.T) {
	tests := []struct {
		name     string
		since    time.Time
		prefix   string
		wantSubs []string
	}{
		{name: "no since no prefix uses unbounded floor", since: time.Time{}, prefix: "", wantSubs: []string{`created >= "2000-01-01"`, "ORDER BY updated ASC"}},
		{name: "since only", since: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), prefix: "", wantSubs: []string{`updated >= "2026-04-01 00:00"`, "ORDER BY updated ASC"}},
		{name: "prefix only", since: time.Time{}, prefix: "project = ANT", wantSubs: []string{"(project = ANT)", "ORDER BY updated ASC"}},
		{name: "both", since: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), prefix: "project = ANT", wantSubs: []string{"(project = ANT)", "AND", `updated >= "2026-04-01 00:00"`, "ORDER BY updated ASC"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			seen := captureJQL(t, tc.since, tc.prefix)
			for _, s := range tc.wantSubs {
				if !strings.Contains(seen, s) {
					t.Errorf("JQL %q missing %q", seen, s)
				}
			}
		})
	}
}

// helpers

func newSinglePageServer(issueJSON string) *httptest.Server {
	body := `{"issues":[` + issueJSON + `],"isLast":true}`
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

// collect drives a connector via StreamSync and gathers items into a slice
// for assertion. Used by tests that don't care about streaming semantics.
func collect(t *testing.T, c *jira.Connector, ctx context.Context, since time.Time) []canonical.WorkItem {
	t.Helper()
	var out []canonical.WorkItem
	if _, err := c.StreamSync(ctx, since, func(item canonical.WorkItem) error {
		out = append(out, item)
		return nil
	}); err != nil {
		t.Fatalf("StreamSync: %v", err)
	}
	return out
}

func captureJQL(t *testing.T, since time.Time, prefix string) string {
	t.Helper()
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if jql, ok := body["jql"].(string); ok {
				seen = jql
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issues":[],"isLast":true}`))
	}))
	defer srv.Close()

	c := jira.New(jira.Credentials{
		Email: "x", APIToken: "y", BaseURL: srv.URL, JQLPrefix: prefix,
	}).WithHTTPClient(srv.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.StreamSync(ctx, since, func(canonical.WorkItem) error { return nil }); err != nil {
		t.Fatalf("StreamSync: %v", err)
	}
	return seen
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
