package canonical

import (
	"time"

	"github.com/google/uuid"
)

type Account struct {
	TenantID    uuid.UUID
	SourceID    string
	Name        string
	Tier        string
	ARR         *float64
	RenewalDate *time.Time
}
