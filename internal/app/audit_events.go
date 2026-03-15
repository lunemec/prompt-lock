package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

const (
	AuditEventRequestReusedActiveLease   = "request_reused_active_lease"
	AuditEventRequestThrottledCooldown   = "request_throttled_cooldown"
	AuditEventRequestThrottledPendingCap = "request_throttled_pending_cap"
	AuditEventSecretAccessStarted        = "secret_access_started"
	AuditEventEnvPathConfirmed           = "env_path_confirmed"
	AuditEventEnvPathRejected            = "env_path_rejected"
)

func (s Service) auditRequestReusedActiveLease(agentID, taskID string, input RequestPolicyInput, lease domain.Lease) error {
	return s.writeAuditEvent(ports.AuditEvent{
		Event:      AuditEventRequestReusedActiveLease,
		Timestamp:  s.now(),
		AgentID:    strings.TrimSpace(agentID),
		TaskID:     strings.TrimSpace(taskID),
		RequestID:  lease.RequestID,
		LeaseToken: lease.Token,
		Metadata: map[string]string{
			"equivalence_key_hash": equivalenceKeyHash(input.EquivalenceKey()),
		},
	})
}

func (s Service) auditRequestThrottledPendingCap(agentID, taskID string, input RequestPolicyInput, retryAfter time.Duration) error {
	return s.writeAuditEvent(ports.AuditEvent{
		Event:     AuditEventRequestThrottledPendingCap,
		Timestamp: s.now(),
		AgentID:   strings.TrimSpace(agentID),
		TaskID:    strings.TrimSpace(taskID),
		Metadata: map[string]string{
			"equivalence_key_hash": equivalenceKeyHash(input.EquivalenceKey()),
			"retry_after_seconds":  strconv.Itoa(retryAfterSeconds(retryAfter)),
		},
	})
}

func (s Service) auditRequestThrottledCooldown(agentID, taskID string, input RequestPolicyInput, retryAfter time.Duration) error {
	return s.writeAuditEvent(ports.AuditEvent{
		Event:     AuditEventRequestThrottledCooldown,
		Timestamp: s.now(),
		AgentID:   strings.TrimSpace(agentID),
		TaskID:    strings.TrimSpace(taskID),
		Metadata: map[string]string{
			"equivalence_key_hash": equivalenceKeyHash(input.EquivalenceKey()),
			"retry_after_seconds":  strconv.Itoa(retryAfterSeconds(retryAfter)),
		},
	})
}

func (s Service) AuditEnvPathConfirmed(agentID, taskID, requestID, envPathOriginal, envPathCanonical string) error {
	return s.writeAuditEvent(envPathConfirmedEvent(s.now(), agentID, taskID, requestID, envPathOriginal, envPathCanonical))
}

func (s Service) AuditEnvPathRejected(agentID, taskID, requestID, envPathOriginal, envPathCanonical, reason string) error {
	return s.writeAuditEvent(envPathRejectedEvent(s.now(), agentID, taskID, requestID, envPathOriginal, envPathCanonical, reason))
}

func (s Service) writeAuditEvent(event ports.AuditEvent) error {
	if s.Audit == nil {
		return nil
	}
	if err := s.Audit.Write(event); err != nil {
		return wrapAuditWriteError(err)
	}
	return nil
}

func wrapAuditWriteError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrAuditWriteFailed) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrAuditWriteFailed, err)
}

func (s Service) auditCritical(event ports.AuditEvent) error {
	if s.Audit == nil {
		return nil
	}
	if err := s.Audit.Write(event); err != nil {
		if s.AuditFailureHandler != nil {
			if handled := s.AuditFailureHandler(err); handled != nil {
				return handled
			}
		}
		return ErrAuditUnavailable
	}
	return nil
}

func envPathConfirmedEvent(now time.Time, agentID, taskID, requestID, envPathOriginal, envPathCanonical string) ports.AuditEvent {
	return ports.AuditEvent{
		Event:     AuditEventEnvPathConfirmed,
		Timestamp: now,
		AgentID:   strings.TrimSpace(agentID),
		TaskID:    strings.TrimSpace(taskID),
		RequestID: strings.TrimSpace(requestID),
		Metadata: map[string]string{
			"env_path_original":  strings.TrimSpace(envPathOriginal),
			"env_path_canonical": strings.TrimSpace(envPathCanonical),
		},
	}
}

func envPathRejectedEvent(now time.Time, agentID, taskID, requestID, envPathOriginal, envPathCanonical, reason string) ports.AuditEvent {
	metadata := map[string]string{
		"env_path_original":  strings.TrimSpace(envPathOriginal),
		"env_path_canonical": strings.TrimSpace(envPathCanonical),
	}
	if trimmedReason := strings.TrimSpace(reason); trimmedReason != "" {
		metadata["reason"] = trimmedReason
	}
	return ports.AuditEvent{
		Event:     AuditEventEnvPathRejected,
		Timestamp: now,
		AgentID:   strings.TrimSpace(agentID),
		TaskID:    strings.TrimSpace(taskID),
		RequestID: strings.TrimSpace(requestID),
		Metadata:  metadata,
	}
}

func secretAccessEvent(event string, now time.Time, lease domain.Lease, secretName string) ports.AuditEvent {
	return ports.AuditEvent{
		Event:      event,
		Timestamp:  now,
		AgentID:    lease.AgentID,
		TaskID:     lease.TaskID,
		RequestID:  lease.RequestID,
		LeaseToken: lease.Token,
		Secret:     secretName,
	}
}

func requestDecisionAuditMetadata(req domain.LeaseRequest, extra map[string]string) map[string]string {
	if strings.TrimSpace(req.EnvPath) == "" && len(extra) == 0 {
		return nil
	}
	metadata := map[string]string{}
	if strings.TrimSpace(req.EnvPath) != "" {
		metadata["env_path_original"] = strings.TrimSpace(req.EnvPath)
		metadata["env_path_canonical"] = strings.TrimSpace(req.EnvPathCanonical)
	}
	for key, value := range extra {
		trimmedValue := strings.TrimSpace(value)
		if trimmedValue == "" {
			continue
		}
		metadata[key] = trimmedValue
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func secretBackendAuditReason(err error) string {
	if err == nil {
		return "backend_error"
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case msg == "":
		return "backend_error"
	case strings.Contains(msg, "status 401"):
		return "http_401"
	case strings.Contains(msg, "status 403"):
		return "http_403"
	case strings.Contains(msg, "status 404"), strings.Contains(msg, "not found"):
		return "not_found"
	case strings.Contains(msg, "status 408"), strings.Contains(msg, "status 429"):
		return "rate_or_timeout"
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "deadline exceeded"):
		return "timeout"
	case strings.Contains(msg, "connection refused"), strings.Contains(msg, "no such host"), strings.Contains(msg, "tls"), strings.Contains(msg, "x509"), strings.Contains(msg, "eof"):
		return "transport_error"
	case strings.Contains(msg, "status 5"):
		return "upstream_5xx"
	case strings.Contains(msg, "status 4"):
		return "upstream_4xx"
	default:
		return "backend_error"
	}
}

func retryAfterSeconds(retryAfter time.Duration) int {
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	seconds := int(math.Ceil(retryAfter.Seconds()))
	if seconds < 1 {
		return 1
	}
	return seconds
}

func equivalenceKeyHash(equivalenceKey string) string {
	trimmed := strings.TrimSpace(equivalenceKey)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	hashed := hex.EncodeToString(sum[:])
	if len(hashed) > 16 {
		return hashed[:16]
	}
	return hashed
}
