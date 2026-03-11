package app

import (
	"crypto/sha256"
	"encoding/hex"
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
	AuditEventEnvPathConfirmed           = "env_path_confirmed"
	AuditEventEnvPathRejected            = "env_path_rejected"
)

func (s Service) auditRequestReusedActiveLease(agentID, taskID string, input RequestPolicyInput, lease domain.Lease) {
	s.writeAuditEvent(ports.AuditEvent{
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

func (s Service) auditRequestThrottledPendingCap(agentID, taskID string, input RequestPolicyInput, retryAfter time.Duration) {
	s.writeAuditEvent(ports.AuditEvent{
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

func (s Service) auditRequestThrottledCooldown(agentID, taskID string, input RequestPolicyInput, retryAfter time.Duration) {
	s.writeAuditEvent(ports.AuditEvent{
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

func (s Service) AuditEnvPathConfirmed(agentID, taskID, requestID, envPathOriginal, envPathCanonical string) {
	s.writeAuditEvent(ports.AuditEvent{
		Event:     AuditEventEnvPathConfirmed,
		Timestamp: s.now(),
		AgentID:   strings.TrimSpace(agentID),
		TaskID:    strings.TrimSpace(taskID),
		RequestID: strings.TrimSpace(requestID),
		Metadata: map[string]string{
			"env_path_original":  strings.TrimSpace(envPathOriginal),
			"env_path_canonical": strings.TrimSpace(envPathCanonical),
		},
	})
}

func (s Service) AuditEnvPathRejected(agentID, taskID, requestID, envPathOriginal, envPathCanonical, reason string) {
	metadata := map[string]string{
		"env_path_original":  strings.TrimSpace(envPathOriginal),
		"env_path_canonical": strings.TrimSpace(envPathCanonical),
	}
	if trimmedReason := strings.TrimSpace(reason); trimmedReason != "" {
		metadata["reason"] = trimmedReason
	}
	s.writeAuditEvent(ports.AuditEvent{
		Event:     AuditEventEnvPathRejected,
		Timestamp: s.now(),
		AgentID:   strings.TrimSpace(agentID),
		TaskID:    strings.TrimSpace(taskID),
		RequestID: strings.TrimSpace(requestID),
		Metadata:  metadata,
	})
}

func (s Service) writeAuditEvent(event ports.AuditEvent) {
	if s.Audit == nil {
		return
	}
	_ = s.Audit.Write(event)
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
