package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type Clock func() time.Time

type Service struct {
	Policy                     domain.Policy
	RequestPolicy              RequestPolicy
	AllowPlaintextSecretReturn bool
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
	return s.RequestLeaseWithIntent(agentID, taskID, reason, ttl, secrets, "", commandFingerprint, workdirFingerprint, envPath)
}

func (s *Service) RequestLeaseWithIntent(agentID, taskID, reason string, ttl int, secrets []string, intent, commandFingerprint, workdirFingerprint, envPath string) (domain.LeaseRequest, error) {
	return s.RequestLeaseWithIntentAndSummary(agentID, taskID, reason, ttl, secrets, intent, commandFingerprint, workdirFingerprint, envPath, "", "")
}

func (s *Service) RequestLeaseWithIntentAndSummary(agentID, taskID, reason string, ttl int, secrets []string, intent, commandFingerprint, workdirFingerprint, envPath, commandSummary, workdirSummary string) (domain.LeaseRequest, error) {
	defer s.lockMutation()()
	return s.requestLeaseUnlocked(agentID, taskID, reason, ttl, secrets, intent, commandFingerprint, workdirFingerprint, envPath, commandSummary, workdirSummary)
}

func (s *Service) requestLeaseUnlocked(agentID, taskID, reason string, ttl int, secrets []string, intent, commandFingerprint, workdirFingerprint, envPath, commandSummary, workdirSummary string) (domain.LeaseRequest, error) {
	if err := s.Policy.ValidateRequest(ttl, secrets); err != nil {
		return domain.LeaseRequest{}, err
	}
	envPath = strings.TrimSpace(envPath)
	envPathCanonical, err := s.canonicalizeApprovedEnvPath(envPath)
	if err != nil {
		return domain.LeaseRequest{}, err
	}
	req := domain.LeaseRequest{
		ID:                 s.NewRequestID(),
		AgentID:            agentID,
		TaskID:             taskID,
		Intent:             strings.TrimSpace(intent),
		Reason:             reason,
		TTLMinutes:         ttl,
		Secrets:            append([]string{}, secrets...),
		CommandFingerprint: commandFingerprint,
		WorkdirFingerprint: workdirFingerprint,
		CommandSummary:     NormalizeRequestSummary(commandSummary, maxRequestCommandSummaryRunes),
		WorkdirSummary:     NormalizeRequestSummary(workdirSummary, maxRequestWorkdirSummaryRunes),
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
		Intent:             req.Intent,
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
	if strings.TrimSpace(req.EnvPath) != "" {
		_ = s.AuditEnvPathConfirmed(req.AgentID, req.TaskID, req.ID, req.EnvPath, req.EnvPathCanonical)
	}
	return lease, nil
}

func (s Service) RejectPlaintextSecretAccess(actorType, actorID string) error {
	if s.AllowPlaintextSecretReturn {
		return nil
	}
	if err := s.auditCritical(ports.AuditEvent{
		Event:     "plaintext_secret_access_blocked",
		Timestamp: s.now(),
		ActorType: actorType,
		ActorID:   actorID,
	}); err != nil {
		return err
	}
	return ErrPlaintextSecretReturnDisabled
}

const (
	maxRequestCommandSummaryRunes = 160
	maxRequestWorkdirSummaryRunes = 120
	maxRequestCommandArgRunes     = 48
	maxRequestCommandArgsPreview  = 6
)

func NormalizeRequestSummary(text string, limit int) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if limit <= 0 {
		limit = maxRequestCommandSummaryRunes
	}
	var b strings.Builder
	b.Grow(len(trimmed))
	for _, r := range trimmed {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteByte(' ')
		case unicode.IsControl(r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	collapsed := strings.Join(strings.Fields(b.String()), " ")
	return truncateRequestSummary(collapsed, limit)
}

func SummarizeCommandArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	limit := maxRequestCommandArgsPreview
	if limit <= 0 {
		limit = 1
	}
	end := len(args)
	if end > limit {
		end = limit
	}
	parts := make([]string, 0, end+1)
	for _, arg := range args[:end] {
		parts = append(parts, summarizeRequestCommandArg(arg))
	}
	if len(args) > limit {
		parts = append(parts, fmt.Sprintf("... (+%d args)", len(args)-limit))
	}
	return NormalizeRequestSummary(strings.Join(parts, " "), maxRequestCommandSummaryRunes)
}

func SummarizeWorkdirPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return NormalizeRequestSummary(filepath.Clean(trimmed), maxRequestWorkdirSummaryRunes)
}

func summarizeRequestCommandArg(arg string) string {
	summary := NormalizeRequestSummary(arg, maxRequestCommandArgRunes)
	if summary == "" {
		return ""
	}
	if strings.ContainsAny(summary, " \t\"") {
		return strconv.Quote(summary)
	}
	return summary
}

func truncateRequestSummary(text string, limit int) string {
	if limit <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

func (s Service) canonicalizeApprovedEnvPath(envPath string) (string, error) {
	trimmedPath := strings.TrimSpace(envPath)
	if trimmedPath == "" {
		return "", nil
	}
	if s.EnvPathSecrets == nil {
		return "", ErrSecretBackendUnavailable
	}
	canonicalPath, err := s.EnvPathSecrets.Canonicalize(trimmedPath)
	if err != nil {
		return "", err
	}
	return normalizeEnvPathCanonical(canonicalPath), nil
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
