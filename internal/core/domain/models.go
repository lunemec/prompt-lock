package domain

import "time"

type RequestStatus string

const (
	RequestPending  RequestStatus = "pending"
	RequestApproved RequestStatus = "approved"
	RequestDenied   RequestStatus = "denied"
)

type LeaseRequest struct {
	ID                 string
	AgentID            string
	TaskID             string
	Reason             string
	TTLMinutes         int
	Secrets            []string
	CommandFingerprint string
	WorkdirFingerprint string
	Status             RequestStatus
	CreatedAt          time.Time
}

type Lease struct {
	Token              string
	RequestID          string
	AgentID            string
	TaskID             string
	Secrets            []string
	CommandFingerprint string
	WorkdirFingerprint string
	ExpiresAt          time.Time
}

func (l Lease) IsExpired(now time.Time) bool {
	return !now.Before(l.ExpiresAt)
}

func (l Lease) Allows(secret string) bool {
	for _, s := range l.Secrets {
		if s == secret {
			return true
		}
	}
	return false
}
