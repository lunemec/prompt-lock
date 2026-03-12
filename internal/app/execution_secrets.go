package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

func (s Service) ResolveExecutionSecrets(leaseToken string, secretNames []string, commandFingerprint, workdirFingerprint string) (map[string]string, error) {
	requested, err := normalizeExecutionSecretNames(secretNames)
	if err != nil {
		return nil, err
	}
	lease, err := s.Leases.GetLease(leaseToken)
	if err != nil {
		return nil, err
	}
	if lease.IsExpired(s.now()) {
		return nil, errors.New("lease expired")
	}
	if lease.CommandFingerprint != "" && lease.CommandFingerprint != commandFingerprint {
		return nil, errors.New("command fingerprint mismatch")
	}
	if lease.WorkdirFingerprint != "" && lease.WorkdirFingerprint != workdirFingerprint {
		return nil, errors.New("workdir fingerprint mismatch")
	}
	for _, secretName := range requested {
		if !lease.Allows(secretName) {
			return nil, fmt.Errorf("secret %q not allowed for this lease", secretName)
		}
	}

	request, err := s.Requests.GetRequest(lease.RequestID)
	if err != nil {
		return nil, err
	}
	if request.Status != domain.RequestApproved {
		return nil, errors.New("request is not approved")
	}

	if strings.TrimSpace(request.EnvPath) == "" {
		values := map[string]string{}
		for _, secretName := range requested {
			value, err := s.AccessSecret(leaseToken, secretName, commandFingerprint, workdirFingerprint)
			if err != nil {
				return nil, err
			}
			values[secretName] = value
		}
		return values, nil
	}

	return s.resolveEnvPathExecutionSecrets(request, lease, requested)
}

func (s Service) resolveEnvPathExecutionSecrets(request domain.LeaseRequest, lease domain.Lease, requested []string) (map[string]string, error) {
	if s.EnvPathSecrets == nil {
		s.AuditEnvPathRejected(request.AgentID, request.TaskID, request.ID, request.EnvPath, request.EnvPathCanonical, "env_path_secret_store_unavailable")
		return nil, ErrSecretBackendUnavailable
	}

	expectedCanonical := filepath.Clean(strings.TrimSpace(request.EnvPathCanonical))
	if expectedCanonical == "" {
		s.AuditEnvPathRejected(request.AgentID, request.TaskID, request.ID, request.EnvPath, request.EnvPathCanonical, "env_path_canonical_missing")
		return nil, errors.New("env path canonical confirmation required")
	}

	resolved, resolvedCanonical, err := s.EnvPathSecrets.Resolve(request.EnvPath, requested)
	if err != nil {
		s.AuditEnvPathRejected(request.AgentID, request.TaskID, request.ID, request.EnvPath, request.EnvPathCanonical, err.Error())
		return nil, ErrSecretBackendUnavailable
	}
	resolvedCanonical = filepath.Clean(strings.TrimSpace(resolvedCanonical))
	if resolvedCanonical == "" || resolvedCanonical != expectedCanonical {
		s.AuditEnvPathRejected(request.AgentID, request.TaskID, request.ID, request.EnvPath, request.EnvPathCanonical, "env_path_canonical_mismatch")
		return nil, errors.New("env path canonical mismatch")
	}

	s.AuditEnvPathConfirmed(request.AgentID, request.TaskID, request.ID, request.EnvPath, resolvedCanonical)
	for _, secretName := range requested {
		s.writeAuditEvent(ports.AuditEvent{
			Event:      "secret_access",
			Timestamp:  s.now(),
			AgentID:    lease.AgentID,
			TaskID:     lease.TaskID,
			RequestID:  lease.RequestID,
			LeaseToken: lease.Token,
			Secret:     secretName,
		})
	}
	return resolved, nil
}

func normalizeExecutionSecretNames(secretNames []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(secretNames))
	for _, secretName := range secretNames {
		trimmed := strings.TrimSpace(secretName)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil, errors.New("secrets are required")
	}
	return out, nil
}
