package app

import "github.com/lunemec/promptlock/internal/core/domain"

type RequestLeaseResult struct {
	Request domain.LeaseRequest
	Lease   domain.Lease
	Reused  bool
}

type leaseLister interface {
	ListLeases() ([]domain.Lease, error)
}

func (s Service) RequestLeaseWithPolicy(agentID, taskID, reason string, ttl int, secrets []string, commandFingerprint, workdirFingerprint string) (RequestLeaseResult, error) {
	if err := s.Policy.ValidateRequest(ttl, secrets); err != nil {
		return RequestLeaseResult{}, err
	}
	policy := DefaultRequestPolicy()
	if policy.EnableActiveLeaseReuse {
		reusableLease, reused, err := s.findEquivalentActiveLease(RequestPolicyInput{
			AgentID:            agentID,
			Secrets:            secrets,
			CommandFingerprint: commandFingerprint,
			WorkdirFingerprint: workdirFingerprint,
		})
		if err != nil {
			return RequestLeaseResult{}, err
		}
		if reused {
			return RequestLeaseResult{Lease: reusableLease, Reused: true}, nil
		}
	}
	created, err := s.RequestLease(agentID, taskID, reason, ttl, secrets, commandFingerprint, workdirFingerprint)
	if err != nil {
		return RequestLeaseResult{}, err
	}
	return RequestLeaseResult{Request: created}, nil
}

func (s Service) findEquivalentActiveLease(input RequestPolicyInput) (domain.Lease, bool, error) {
	lister, ok := s.Leases.(leaseLister)
	if !ok {
		return domain.Lease{}, false, nil
	}
	leases, err := lister.ListLeases()
	if err != nil {
		return domain.Lease{}, false, err
	}

	now := s.now()
	target := input.EquivalenceKey()
	for _, lease := range leases {
		if lease.IsExpired(now) {
			continue
		}
		candidate := RequestPolicyInput{
			AgentID:            lease.AgentID,
			Secrets:            lease.Secrets,
			CommandFingerprint: lease.CommandFingerprint,
			WorkdirFingerprint: lease.WorkdirFingerprint,
		}
		if candidate.EquivalenceKey() == target {
			return lease, true, nil
		}
	}

	return domain.Lease{}, false, nil
}
