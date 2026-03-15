package app

import (
	"errors"

	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

func (s Service) CancelRequestByAgent(requestID, agentID, reason string) (domain.LeaseRequest, error) {
	req, err := s.Requests.GetRequest(requestID)
	if err != nil {
		return domain.LeaseRequest{}, err
	}
	if agentID != "" && req.AgentID != agentID {
		return domain.LeaseRequest{}, ErrRequestNotOwned
	}
	if req.Status != domain.RequestPending {
		return domain.LeaseRequest{}, errors.New("request is not pending")
	}
	original := req
	req.Status = domain.RequestDenied
	if err := s.Requests.UpdateRequest(req); err != nil {
		return domain.LeaseRequest{}, err
	}
	if err := s.commitRequestLeaseState(); err != nil {
		return domain.LeaseRequest{}, wrapRollbackError(err, s.rollbackRequestLeaseMutation(func() error {
			return s.Requests.UpdateRequest(original)
		}))
	}
	if err := s.auditCritical(ports.AuditEvent{
		Event:     "request_cancelled_by_agent",
		Timestamp: s.now(),
		AgentID:   req.AgentID,
		TaskID:    req.TaskID,
		RequestID: req.ID,
		Metadata:  map[string]string{"reason": reason},
	}); err != nil {
		return domain.LeaseRequest{}, wrapRollbackError(err, s.rollbackRequestLeaseMutation(func() error {
			return s.Requests.UpdateRequest(original)
		}))
	}
	return req, nil
}
