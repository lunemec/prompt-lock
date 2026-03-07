package app

import (
	"errors"

	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

func (s Service) DenyRequest(requestID string, reason string) (domain.LeaseRequest, error) {
	req, err := s.Requests.GetRequest(requestID)
	if err != nil {
		return domain.LeaseRequest{}, err
	}
	if req.Status != domain.RequestPending {
		return domain.LeaseRequest{}, errors.New("request is not pending")
	}
	req.Status = domain.RequestDenied
	if err := s.Requests.UpdateRequest(req); err != nil {
		return domain.LeaseRequest{}, err
	}
	_ = s.Audit.Write(ports.AuditEvent{
		Event:     "request_denied",
		Timestamp: s.now(),
		AgentID:   req.AgentID,
		TaskID:    req.TaskID,
		RequestID: req.ID,
		Metadata:  map[string]string{"reason": reason},
	})
	return req, nil
}
