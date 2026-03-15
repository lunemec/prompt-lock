package app

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type Clock func() time.Time

type Service struct {
	Policy                     domain.Policy
	RequestPolicy              RequestPolicy
	MutationLock               sync.Locker
	Requests                   ports.RequestStore
	Leases                     ports.LeaseStore
	RequestLeaseStateCommitter ports.RequestLeaseStateCommitter
	Secrets                    ports.SecretStore
	EnvPathSecrets             ports.EnvPathSecretStore
	Audit                      ports.AuditSink
	AuditFailureHandler        func(error) error
	Now                        Clock
	NewRequestID               func() string
	NewLeaseTok                func() string
}

func (s Service) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func (s Service) requestPolicy() RequestPolicy {
	if s.RequestPolicy == (RequestPolicy{}) {
		return DefaultRequestPolicy()
	}
	return s.RequestPolicy.Normalize()
}

func (s *Service) lockMutation() func() {
	if s == nil || s.MutationLock == nil {
		return func() {}
	}
	s.MutationLock.Lock()
	return s.MutationLock.Unlock
}

func (s *Service) commitRequestLeaseState() error {
	if s.RequestLeaseStateCommitter == nil {
		return nil
	}
	return s.RequestLeaseStateCommitter.CommitRequestLeaseState()
}

func (s *Service) RequestLease(agentID, taskID, reason string, ttl int, secrets []string, commandFingerprint, workdirFingerprint, envPath, envPathCanonical string) (domain.LeaseRequest, error) {
	defer s.lockMutation()()
	return s.requestLeaseUnlocked(agentID, taskID, reason, ttl, secrets, commandFingerprint, workdirFingerprint, envPath, envPathCanonical)
}

func (s *Service) requestLeaseUnlocked(agentID, taskID, reason string, ttl int, secrets []string, commandFingerprint, workdirFingerprint, envPath, envPathCanonical string) (domain.LeaseRequest, error) {
	if err := s.Policy.ValidateRequest(ttl, secrets); err != nil {
		return domain.LeaseRequest{}, err
	}
	envPath = strings.TrimSpace(envPath)
	envPathCanonical = normalizeEnvPathCanonical(envPathCanonical)
	req := domain.LeaseRequest{
		ID:                 s.NewRequestID(),
		AgentID:            agentID,
		TaskID:             taskID,
		Reason:             reason,
		TTLMinutes:         ttl,
		Secrets:            append([]string{}, secrets...),
		CommandFingerprint: commandFingerprint,
		WorkdirFingerprint: workdirFingerprint,
		EnvPath:            envPath,
		EnvPathCanonical:   envPathCanonical,
		Status:             domain.RequestPending,
		CreatedAt:          s.now(),
	}
	if err := s.Requests.SaveRequest(req); err != nil {
		return domain.LeaseRequest{}, err
	}
	if err := s.commitRequestLeaseState(); err != nil {
		return domain.LeaseRequest{}, wrapRollbackError(err, s.rollbackRequestLeaseMutation(func() error {
			return rollbackCreatedRequest(s.Requests, req.ID)
		}))
	}
	if err := s.auditCritical(ports.AuditEvent{Event: "request_created", Timestamp: s.now(), AgentID: agentID, TaskID: taskID, RequestID: req.ID}); err != nil {
		return domain.LeaseRequest{}, wrapRollbackError(err, s.rollbackRequestLeaseMutation(func() error {
			return rollbackCreatedRequest(s.Requests, req.ID)
		}))
	}
	return req, nil
}

func (s *Service) ApproveRequest(requestID string, ttlMinutes int) (domain.Lease, error) {
	return s.ApproveRequestWithActor(requestID, ttlMinutes, "", "")
}

func (s *Service) ApproveRequestWithActor(requestID string, ttlMinutes int, actorType, actorID string) (domain.Lease, error) {
	defer s.lockMutation()()
	return s.approveRequestWithActorUnlocked(requestID, ttlMinutes, actorType, actorID)
}

func (s *Service) approveRequestWithActorUnlocked(requestID string, ttlMinutes int, actorType, actorID string) (domain.Lease, error) {
	req, err := s.Requests.GetRequest(requestID)
	if err != nil {
		return domain.Lease{}, err
	}
	if req.Status != domain.RequestPending {
		return domain.Lease{}, errors.New("request is not pending")
	}
	if ttlMinutes == 0 {
		ttlMinutes = req.TTLMinutes
	}
	if err := s.Policy.ValidateRequest(ttlMinutes, req.Secrets); err != nil {
		return domain.Lease{}, err
	}
	original := req
	req.Status = domain.RequestApproved
	if err := s.Requests.UpdateRequest(req); err != nil {
		return domain.Lease{}, err
	}
	lease := domain.Lease{
		Token:              s.NewLeaseTok(),
		RequestID:          req.ID,
		AgentID:            req.AgentID,
		TaskID:             req.TaskID,
		Secrets:            append([]string{}, req.Secrets...),
		CommandFingerprint: req.CommandFingerprint,
		WorkdirFingerprint: req.WorkdirFingerprint,
		ExpiresAt:          s.now().Add(time.Duration(ttlMinutes) * time.Minute),
	}
	if err := s.Leases.SaveLease(lease); err != nil {
		if rollbackErr := s.Requests.UpdateRequest(original); rollbackErr != nil {
			return domain.Lease{}, fmt.Errorf("save lease: %w (request rollback failed: %v)", err, rollbackErr)
		}
		return domain.Lease{}, err
	}
	if err := s.commitRequestLeaseState(); err != nil {
		return domain.Lease{}, wrapRollbackError(err, s.rollbackRequestLeaseMutation(
			func() error { return rollbackCreatedLease(s.Leases, lease.Token) },
			func() error { return s.Requests.UpdateRequest(original) },
		))
	}
	if err := s.auditCritical(ports.AuditEvent{
		Event:      "request_approved",
		Timestamp:  s.now(),
		ActorType:  actorType,
		ActorID:    actorID,
		AgentID:    req.AgentID,
		TaskID:     req.TaskID,
		RequestID:  req.ID,
		LeaseToken: lease.Token,
		Metadata:   requestDecisionAuditMetadata(req, nil),
	}); err != nil {
		return domain.Lease{}, wrapRollbackError(err, s.rollbackRequestLeaseMutation(
			func() error { return rollbackCreatedLease(s.Leases, lease.Token) },
			func() error { return s.Requests.UpdateRequest(original) },
		))
	}
	return lease, nil
}

func (s Service) AccessSecret(leaseToken, secretName, commandFingerprint, workdirFingerprint string) (string, error) {
	lease, err := s.getLeaseOwnedByAgent(leaseToken, "")
	if err != nil {
		return "", err
	}
	return s.accessSecretForLease(lease, secretName, commandFingerprint, workdirFingerprint)
}

func (s Service) AccessSecretByAgent(agentID, leaseToken, secretName, commandFingerprint, workdirFingerprint string) (string, error) {
	lease, err := s.getLeaseOwnedByAgent(leaseToken, agentID)
	if err != nil {
		return "", err
	}
	return s.accessSecretForLease(lease, secretName, commandFingerprint, workdirFingerprint)
}

func (s Service) accessSecretForLease(lease domain.Lease, secretName, commandFingerprint, workdirFingerprint string) (string, error) {
	if lease.IsExpired(s.now()) {
		return "", errors.New("lease expired")
	}
	if !lease.Allows(secretName) {
		return "", fmt.Errorf("secret %q not allowed for this lease", secretName)
	}
	if lease.CommandFingerprint != "" && lease.CommandFingerprint != commandFingerprint {
		return "", errors.New("command fingerprint mismatch")
	}
	if lease.WorkdirFingerprint != "" && lease.WorkdirFingerprint != workdirFingerprint {
		return "", errors.New("workdir fingerprint mismatch")
	}
	if err := s.auditCritical(secretAccessEvent(AuditEventSecretAccessStarted, s.now(), lease, secretName)); err != nil {
		return "", err
	}
	val, err := s.Secrets.GetSecret(secretName)
	if err != nil {
		reason := secretBackendAuditReason(err)
		s.writeAuditEvent(ports.AuditEvent{Event: "secret_backend_error", Timestamp: s.now(), AgentID: lease.AgentID, TaskID: lease.TaskID, RequestID: lease.RequestID, LeaseToken: lease.Token, Secret: secretName, Metadata: map[string]string{"reason": reason}})
		return "", ErrSecretBackendUnavailable
	}
	if err := s.auditCritical(secretAccessEvent("secret_access", s.now(), lease, secretName)); err != nil {
		return "", err
	}
	return val, nil
}
