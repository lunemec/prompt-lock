package app

import (
	"errors"

	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

func (s *Service) DenyRequest(requestID string, reason string) (domain.LeaseRequest, error) {
	return s.DenyRequestWithActor(requestID, reason, "", "")
}

func (s *Service) DenyRequestWithActor(requestID string, reason string, actorType, actorID string) (domain.LeaseRequest, error) {
	defer s.lockMutation()()
	return s.denyRequestWithActorUnlocked(requestID, reason, actorType, actorID)
}

func (s *Service) denyRequestWithActorUnlocked(requestID string, reason string, actorType, actorID string) (domain.LeaseRequest, error) {
	req, err := s.Requests.GetRequest(requestID)
	if err != nil {
		return domain.LeaseRequest{}, err
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
		Event:     "request_denied",
		Timestamp: s.now(),
		ActorType: actorType,
		ActorID:   actorID,
		AgentID:   req.AgentID,
		TaskID:    req.TaskID,
		RequestID: req.ID,
		Metadata:  requestDecisionAuditMetadata(req, map[string]string{"reason": reason}),
	}); err != nil {
		return domain.LeaseRequest{}, wrapRollbackError(err, s.rollbackRequestLeaseMutation(func() error {
			return s.Requests.UpdateRequest(original)
		}))
	}
	return req, nil
}
