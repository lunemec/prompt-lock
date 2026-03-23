package app

import (
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/core/domain"
)

type RequestLeaseResult struct {
	Request domain.LeaseRequest
	Lease   domain.Lease
	Reused  bool
}

type leaseLister interface {
	ListLeases() ([]domain.Lease, error)
}

func (s *Service) RequestLeaseWithPolicy(agentID, taskID, reason string, ttl int, secrets []string, commandFingerprint, workdirFingerprint, envPath, envPathCanonical string) (RequestLeaseResult, error) {
	return s.RequestLeaseWithPolicyAndIntent(agentID, taskID, reason, ttl, secrets, "", commandFingerprint, workdirFingerprint, envPath)
}

func (s *Service) RequestLeaseWithPolicyAndIntent(agentID, taskID, reason string, ttl int, secrets []string, intent, commandFingerprint, workdirFingerprint, envPath string) (RequestLeaseResult, error) {
	return s.RequestLeaseWithPolicyAndIntentAndSummary(agentID, taskID, reason, ttl, secrets, intent, commandFingerprint, workdirFingerprint, envPath, "", "")
}

func (s *Service) RequestLeaseWithPolicyAndIntentAndSummary(agentID, taskID, reason string, ttl int, secrets []string, intent, commandFingerprint, workdirFingerprint, envPath, commandSummary, workdirSummary string) (RequestLeaseResult, error) {
	defer s.lockMutation()()
	if err := s.Policy.ValidateRequest(ttl, secrets); err != nil {
		return RequestLeaseResult{}, err
	}

	policy := s.requestPolicy()
	input := RequestPolicyInput{
		AgentID:            agentID,
		Intent:             strings.TrimSpace(intent),
		Secrets:            secrets,
		CommandFingerprint: commandFingerprint,
		WorkdirFingerprint: workdirFingerprint,
	}
	envPathRequested := strings.TrimSpace(envPath) != ""

	if policy.EnableActiveLeaseReuse && !envPathRequested {
		reusableLease, reused, err := s.findEquivalentActiveLease(input)
		if err != nil {
			return RequestLeaseResult{}, err
		}
		if reused {
			if err := s.auditRequestReusedActiveLease(agentID, taskID, input, reusableLease); err != nil {
				return RequestLeaseResult{}, err
			}
			return RequestLeaseResult{Lease: reusableLease, Reused: true}, nil
		}
	}

	pending, err := s.ListPendingRequests()
	if err != nil {
		return RequestLeaseResult{}, err
	}
	if countPendingRequestsForAgent(agentID, pending) >= policy.MaxPendingPerAgent {
		if err := s.auditRequestThrottledPendingCap(agentID, taskID, input, policy.IdenticalRequestCooldown); err != nil {
			return RequestLeaseResult{}, err
		}
		return RequestLeaseResult{}, NewRequestThrottleError(RequestThrottleReasonPendingCap, policy.IdenticalRequestCooldown)
	}

	if retryAfter, throttled := s.equivalentRequestCooldown(input, pending, policy.IdenticalRequestCooldown); throttled {
		if err := s.auditRequestThrottledCooldown(agentID, taskID, input, retryAfter); err != nil {
			return RequestLeaseResult{}, err
		}
		return RequestLeaseResult{}, NewRequestThrottleError(RequestThrottleReasonCooldown, retryAfter)
	}

	created, err := s.requestLeaseUnlocked(agentID, taskID, reason, ttl, secrets, intent, commandFingerprint, workdirFingerprint, envPath, commandSummary, workdirSummary)
	if err != nil {
		return RequestLeaseResult{}, err
	}
	return RequestLeaseResult{Request: created}, nil
}

func (s Service) ListPendingRequests() ([]domain.LeaseRequest, error) {
	return s.Requests.ListPendingRequests()
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
			Intent:             lease.Intent,
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

func countPendingRequestsForAgent(agentID string, pending []domain.LeaseRequest) int {
	target := strings.TrimSpace(agentID)
	count := 0
	for _, req := range pending {
		if strings.TrimSpace(req.AgentID) == target {
			count++
		}
	}
	return count
}

func (s Service) equivalentRequestCooldown(input RequestPolicyInput, pending []domain.LeaseRequest, cooldown time.Duration) (time.Duration, bool) {
	if cooldown <= 0 {
		return 0, false
	}

	now := s.now()
	target := input.EquivalenceKey()
	maxRetryAfter := time.Duration(0)
	for _, req := range pending {
		candidate := RequestPolicyInput{
			AgentID:            req.AgentID,
			Intent:             req.Intent,
			Secrets:            req.Secrets,
			CommandFingerprint: req.CommandFingerprint,
			WorkdirFingerprint: req.WorkdirFingerprint,
		}
		if candidate.EquivalenceKey() != target {
			continue
		}

		elapsed := now.Sub(req.CreatedAt)
		if elapsed < 0 {
			elapsed = 0
		}
		if elapsed >= cooldown {
			continue
		}

		retryAfter := cooldown - elapsed
		if retryAfter > maxRetryAfter {
			maxRetryAfter = retryAfter
		}
	}

	if maxRetryAfter <= 0 {
		return 0, false
	}
	return maxRetryAfter, true
}
