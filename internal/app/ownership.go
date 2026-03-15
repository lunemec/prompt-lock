package app

import (
	"errors"
	"strings"

	"github.com/lunemec/promptlock/internal/core/domain"
)

func (s Service) RequestStatusByAgent(requestID, agentID string) (domain.LeaseRequest, error) {
	return s.getRequestOwnedByAgent(requestID, agentID)
}

func (s Service) LeaseByRequestForAgent(requestID, agentID string) (domain.Lease, error) {
	if _, err := s.getRequestOwnedByAgent(requestID, agentID); err != nil {
		return domain.Lease{}, err
	}
	lease, err := s.Leases.GetLeaseByRequestID(requestID)
	if err != nil {
		return domain.Lease{}, err
	}
	if agentID != "" && lease.AgentID != agentID {
		return domain.Lease{}, ErrLeaseNotOwned
	}
	return lease, nil
}

func (s Service) ApprovedLeaseIntentByAgent(leaseToken, agentID string) (string, error) {
	lease, err := s.getLeaseOwnedByAgent(leaseToken, agentID)
	if err != nil {
		return "", err
	}
	req, err := s.getRequestOwnedByAgent(lease.RequestID, agentID)
	if err != nil {
		return "", err
	}
	if req.Status != domain.RequestApproved {
		return "", errors.New("request is not approved")
	}
	if intent := strings.TrimSpace(lease.Intent); intent != "" {
		return intent, nil
	}
	return strings.TrimSpace(req.Intent), nil
}

func (s Service) getRequestOwnedByAgent(requestID, agentID string) (domain.LeaseRequest, error) {
	req, err := s.Requests.GetRequest(requestID)
	if err != nil {
		return domain.LeaseRequest{}, err
	}
	if agentID != "" && req.AgentID != agentID {
		return domain.LeaseRequest{}, ErrRequestNotOwned
	}
	return req, nil
}

func (s Service) getLeaseOwnedByAgent(leaseToken, agentID string) (domain.Lease, error) {
	lease, err := s.Leases.GetLease(leaseToken)
	if err != nil {
		return domain.Lease{}, err
	}
	if agentID != "" && lease.AgentID != agentID {
		return domain.Lease{}, ErrLeaseNotOwned
	}
	return lease, nil
}
