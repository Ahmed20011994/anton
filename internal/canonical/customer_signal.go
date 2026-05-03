package canonical

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CustomerSignal is the canonical model for support tool data
// (Zendesk, Intercom, Freshdesk, etc.).
//
// This struct is a placeholder for the upcoming Zendesk slice.
// The schema migration for customer_signals is intentionally
// deferred until that slice — only the type exists here so
// connector interfaces can compile.
type CustomerSignal struct {
	TenantID            uuid.UUID
	SourceID            string
	SourceType          string
	SignalType          string
	Subject             string
	Body                string
	Status              string
	Priority            string
	CustomerIdentifier  string
	AccountIdentifier   string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ResolvedAt          *time.Time
	SatisfactionScore   *float64
	EscalationLevel     int
	ReopenCount         int
	ConversationThread  json.RawMessage
	Tags                []string
	RawPayload          json.RawMessage
	ContentHash         string
}
