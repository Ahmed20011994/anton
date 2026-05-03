package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ahmed20011994/anton/internal/canonical"
	"github.com/Ahmed20011994/anton/internal/connector"
)

const (
	sourceType     = "jira"
	pageSize       = 100
	defaultTimeout = 30 * time.Second
	maxRetries     = 3
)

type Credentials struct {
	Email     string `json:"email"`
	APIToken  string `json:"api_token"`
	BaseURL   string `json:"base_url"`
	JQLPrefix string `json:"jql_prefix,omitempty"`
}

func (c Credentials) validate() error {
	if c.Email == "" {
		return fmt.Errorf("credentials: email required")
	}
	if c.APIToken == "" {
		return fmt.Errorf("credentials: api_token required")
	}
	if c.BaseURL == "" {
		return fmt.Errorf("credentials: base_url required")
	}
	return nil
}

type Connector struct {
	creds Credentials
	http  *http.Client
	base  string
}

func New(creds Credentials) *Connector {
	return &Connector{
		creds: creds,
		http:  &http.Client{Timeout: defaultTimeout},
		base:  strings.TrimRight(creds.BaseURL, "/"),
	}
}

// WithHTTPClient swaps the underlying *http.Client (used by tests).
func (c *Connector) WithHTTPClient(client *http.Client) *Connector {
	c.http = client
	return c
}

func (c *Connector) SourceType() string { return sourceType }

func (c *Connector) authHeader() string {
	raw := c.creds.Email + ":" + c.creds.APIToken
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

func (c *Connector) Connect(ctx context.Context) error {
	if err := c.creds.validate(); err != nil {
		return fmt.Errorf("Connect: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/rest/api/3/myself", nil)
	if err != nil {
		return fmt.Errorf("Connect: build req: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Connect: do: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("Connect: %w (status %d)", connector.ErrAuth, resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Connect: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *Connector) HealthCheck(ctx context.Context) connector.HealthStatus {
	start := time.Now()
	err := c.Connect(ctx)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return connector.HealthStatus{OK: false, LatencyMs: latency, Detail: err.Error()}
	}
	return connector.HealthStatus{OK: true, LatencyMs: latency}
}

func (c *Connector) StreamSync(ctx context.Context, since time.Time, fn func(canonical.WorkItem) error) (int, error) {
	return c.fetch(ctx, since, fn)
}

func (c *Connector) StreamBackfill(ctx context.Context, from time.Time, fn func(canonical.WorkItem) error) (int, error) {
	return c.fetch(ctx, from, fn)
}

func (c *Connector) fetch(ctx context.Context, since time.Time, fn func(canonical.WorkItem) error) (int, error) {
	fetchErrors := 0
	jql := buildJQL(since, c.creds.JQLPrefix)
	nextPageToken := ""
	for {
		body := map[string]any{
			"jql":        jql,
			"maxResults": pageSize,
			"fields":     []string{"*all"},
		}
		if nextPageToken != "" {
			body["nextPageToken"] = nextPageToken
		}

		page, err := c.searchPage(ctx, body)
		if err != nil {
			return fetchErrors, fmt.Errorf("fetch: %w", err)
		}

		for _, raw := range page.Issues {
			item, err := mapIssueToWorkItem(raw)
			if err != nil {
				fetchErrors++
				continue
			}
			if err := fn(item); err != nil {
				return fetchErrors, fmt.Errorf("fetch: callback: %w", err)
			}
		}

		if page.IsLast || page.NextPageToken == "" {
			break
		}
		nextPageToken = page.NextPageToken
	}

	return fetchErrors, nil
}

func buildJQL(since time.Time, prefix string) string {
	var clauses []string
	if prefix != "" {
		clauses = append(clauses, "("+prefix+")")
	}
	switch {
	case !since.IsZero():
		clauses = append(clauses, fmt.Sprintf(`updated >= "%s"`, since.UTC().Format("2006-01-02 15:04")))
	case prefix == "":
		// Atlassian's /search/jql endpoint rejects fully unbounded queries.
		// Use an effectively-unbounded floor so every issue passes through
		// without imposing an arbitrary window.
		clauses = append(clauses, `created >= "2000-01-01"`)
	}
	return strings.Join(clauses, " AND ") + " ORDER BY updated ASC"
}

func (c *Connector) searchPage(ctx context.Context, body map[string]any) (searchResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return searchResponse{}, fmt.Errorf("searchPage: marshal: %w", err)
	}

	var resp searchResponse
	if err := c.doWithRetry(ctx, http.MethodPost, "/rest/api/3/search/jql", payload, &resp); err != nil {
		return searchResponse{}, fmt.Errorf("searchPage: %w", err)
	}
	return resp, nil
}

func (c *Connector) doWithRetry(ctx context.Context, method, path string, body []byte, out any) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt) * time.Second
			if rl, ok := lastErr.(*connector.RateLimitedError); ok && rl.RetryAfter > 0 {
				backoff = rl.RetryAfter
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("doWithRetry: ctx: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("doWithRetry: build req: %w", err)
		}
		req.Header.Set("Authorization", c.authHeader())
		req.Header.Set("Accept", "application/json")
		if len(body) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		status := resp.StatusCode
		switch {
		case status == http.StatusUnauthorized || status == http.StatusForbidden:
			_ = resp.Body.Close()
			return fmt.Errorf("doWithRetry: %w (status %d)", connector.ErrAuth, status)
		case status == http.StatusTooManyRequests:
			ra := parseRetryAfter(resp.Header.Get("Retry-After"))
			_ = resp.Body.Close()
			lastErr = &connector.RateLimitedError{RetryAfter: ra}
			continue
		case status >= 500:
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("doWithRetry: status %d: %s", status, string(b))
			continue
		case status >= 400:
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			return fmt.Errorf("doWithRetry: status %d: %s", status, string(b))
		}

		err = func() error {
			defer resp.Body.Close()
			if out != nil {
				if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
					return fmt.Errorf("decode: %w", err)
				}
			}
			return nil
		}()
		if err != nil {
			// Decode failures (typically truncated bodies / unexpected EOF
			// from upstream proxies dropping connections) are transient.
			// Retry until maxRetries is exhausted.
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("doWithRetry: max retries exceeded: %w", lastErr)
}

func parseRetryAfter(h string) time.Duration {
	if h == "" {
		return 2 * time.Second
	}
	if secs, err := strconv.Atoi(h); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 2 * time.Second
}
