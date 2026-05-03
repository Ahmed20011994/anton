package canonical

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type WorkItem struct {
	TenantID              uuid.UUID
	SourceID              string
	SourceType            string
	ItemType              string
	Title                 string
	Description           string
	Status                string
	StatusCategory        string
	Priority              string
	Assignees             []string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	ClosedAt              *time.Time
	ReopenCount           int
	Comments              json.RawMessage
	LinkedCustomerSignals []string
	Version               *string
	RawPayload            json.RawMessage
	ContentHash           string
}
