package jira

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Ahmed20011994/anton/internal/canonical"
)

// Jira API DTOs — only the fields we map.

type issue struct {
	ID     string         `json:"id"`
	Key    string         `json:"key"`
	Fields map[string]any `json:"fields"`
}

type searchResponse struct {
	Issues        []json.RawMessage `json:"issues"`
	NextPageToken string            `json:"nextPageToken,omitempty"`
	IsLast        bool              `json:"isLast"`
}

// statusCategoryMap maps Jira's statusCategory.key → canonical status_category.
// "new"           → todo
// "indeterminate" → in_progress
// "done"          → done
// anything else   → "" (unknown)
var statusCategoryMap = map[string]string{
	"new":           "todo",
	"indeterminate": "in_progress",
	"done":          "done",
}

// itemTypeMap maps lowercased Jira issuetype.name → canonical item_type.
var itemTypeMap = map[string]string{
	"bug":         "bug",
	"task":        "task",
	"sub-task":    "task",
	"subtask":     "task",
	"story":       "feature",
	"new feature": "feature",
	"feature":     "feature",
	"epic":        "epic",
}

func mapItemType(jiraName string) string {
	key := strings.ToLower(strings.TrimSpace(jiraName))
	if v, ok := itemTypeMap[key]; ok {
		return v
	}
	return "task"
}

func mapStatusCategory(key string) string {
	if v, ok := statusCategoryMap[strings.ToLower(key)]; ok {
		return v
	}
	return ""
}

// mapIssueToWorkItem converts a raw Jira issue JSON into a canonical.WorkItem.
// Returns the WorkItem and an error if mandatory fields are missing.
func mapIssueToWorkItem(raw json.RawMessage) (canonical.WorkItem, error) {
	var iss issue
	if err := json.Unmarshal(raw, &iss); err != nil {
		return canonical.WorkItem{}, fmt.Errorf("mapIssueToWorkItem: unmarshal: %w", err)
	}
	if iss.Key == "" {
		return canonical.WorkItem{}, fmt.Errorf("mapIssueToWorkItem: issue missing key")
	}

	w := canonical.WorkItem{
		SourceID:   iss.Key,
		SourceType: "jira",
		RawPayload: raw,
	}

	if iss.Fields == nil {
		w.Assignees = []string{}
		w.LinkedCustomerSignals = []string{}
		w.Comments = json.RawMessage(`[]`)
		return w, nil
	}

	w.Title = stringField(iss.Fields, "summary")
	w.Description = descriptionToText(iss.Fields["description"])

	if status, ok := iss.Fields["status"].(map[string]any); ok {
		w.Status = stringField(status, "name")
		if cat, ok := status["statusCategory"].(map[string]any); ok {
			w.StatusCategory = mapStatusCategory(stringField(cat, "key"))
		}
	}

	if issuetype, ok := iss.Fields["issuetype"].(map[string]any); ok {
		w.ItemType = mapItemType(stringField(issuetype, "name"))
	}

	if priority, ok := iss.Fields["priority"].(map[string]any); ok {
		w.Priority = strings.ToLower(stringField(priority, "name"))
	}

	w.Assignees = collectAssignees(iss.Fields)
	w.CreatedAt = parseJiraTime(stringField(iss.Fields, "created"))
	w.UpdatedAt = parseJiraTime(stringField(iss.Fields, "updated"))
	if resolved := parseJiraTime(stringField(iss.Fields, "resolutiondate")); !resolved.IsZero() {
		w.ClosedAt = &resolved
	}

	w.LinkedCustomerSignals = collectCustomerLinks(iss.Fields["issuelinks"])
	w.Comments = collectComments(iss.Fields["comment"])
	w.Version = collectFixVersion(iss.Fields["fixVersions"])
	w.ContentHash = hashContent(w.Title, w.Description)
	return w, nil
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func collectAssignees(fields map[string]any) []string {
	out := []string{}
	if a, ok := fields["assignee"].(map[string]any); ok {
		if name := stringField(a, "displayName"); name != "" {
			out = append(out, name)
		} else if email := stringField(a, "emailAddress"); email != "" {
			out = append(out, email)
		}
	}
	return out
}

// descriptionToText extracts a plain-text-ish representation of a Jira description.
// Jira Cloud v3 returns ADF (Atlassian Document Format) as a JSON object;
// older APIs return a plain string. We accept both.
func descriptionToText(v any) string {
	switch d := v.(type) {
	case nil:
		return ""
	case string:
		return d
	default:
		// ADF — marshal to compact JSON; intelligence layer will flatten later.
		b, err := json.Marshal(d)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// collectCustomerLinks returns the source IDs of customer/support-linked issues.
// Per the doc: linkType.name matching "customer" or "support" (case-insensitive).
func collectCustomerLinks(v any) []string {
	links, ok := v.([]any)
	if !ok {
		return []string{}
	}
	out := []string{}
	for _, raw := range links {
		l, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		t, ok := l["type"].(map[string]any)
		if !ok {
			continue
		}
		name := strings.ToLower(stringField(t, "name"))
		if !strings.Contains(name, "customer") && !strings.Contains(name, "support") {
			continue
		}
		// Either inwardIssue or outwardIssue carries the linked target.
		for _, side := range []string{"inwardIssue", "outwardIssue"} {
			if obj, ok := l[side].(map[string]any); ok {
				if k := stringField(obj, "key"); k != "" {
					out = append(out, k)
				}
			}
		}
	}
	return out
}

func collectComments(v any) json.RawMessage {
	c, ok := v.(map[string]any)
	if !ok {
		return json.RawMessage(`[]`)
	}
	arr, ok := c["comments"].([]any)
	if !ok {
		return json.RawMessage(`[]`)
	}
	b, err := json.Marshal(arr)
	if err != nil {
		return json.RawMessage(`[]`)
	}
	return b
}

func collectFixVersion(v any) *string {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	if first, ok := arr[0].(map[string]any); ok {
		if name := stringField(first, "name"); name != "" {
			return &name
		}
	}
	return nil
}

func parseJiraTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Jira returns timestamps like "2024-01-15T10:30:00.000+0000".
	formats := []string{
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05-0700",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func hashContent(title, description string) string {
	sum := sha256.Sum256([]byte(title + "\n" + description))
	return hex.EncodeToString(sum[:])
}
