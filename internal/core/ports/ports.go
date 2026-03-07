package ports

import (
	"time"

	"github.com/lunemec/promptlock/internal/core/domain"
)

type RequestStore interface {
	SaveRequest(req domain.LeaseRequest) error
	GetRequest(id string) (domain.LeaseRequest, error)
	UpdateRequest(req domain.LeaseRequest) error
}

type LeaseStore interface {
	SaveLease(lease domain.Lease) error
	GetLease(token string) (domain.Lease, error)
}

type SecretStore interface {
	GetSecret(name string) (string, error)
}

type AuditSink interface {
	Write(event AuditEvent) error
}

type AuditEvent struct {
	Event      string            `json:"event"`
	Timestamp  time.Time         `json:"timestamp"`
	AgentID    string            `json:"agent_id,omitempty"`
	TaskID     string            `json:"task_id,omitempty"`
	RequestID  string            `json:"request_id,omitempty"`
	LeaseToken string            `json:"lease_token,omitempty"`
	Secret     string            `json:"secret,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}
