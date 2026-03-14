package app

import "github.com/lunemec/promptlock/internal/core/domain"

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
