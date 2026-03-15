package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type AuthStore interface {
	SaveBootstrap(t auth.BootstrapToken)
	ConsumeBootstrap(token, containerID string, now time.Time) (auth.BootstrapToken, error)
	SaveGrant(g auth.PairingGrant)
	GetGrant(id string) (auth.PairingGrant, error)
	UpdateGrant(g auth.PairingGrant)
	SaveSession(tok auth.SessionToken)
	RevokeGrant(id string) error
	RevokeSession(token string) error
	Snapshot() auth.StoreSnapshot
	Restore(state auth.StoreSnapshot)
}

type AuthLifecycleConfig struct {
	BootstrapTokenTTLSeconds int
	GrantIdleTimeoutMinutes  int
	GrantAbsoluteMaxMinutes  int
	SessionTTLMinutes        int
}

type AuthLifecycle struct {
	Store               AuthStore
	Audit               ports.AuditSink
	AuditFailureHandler func(error) error
	MutationLock        sync.Locker
	Now                 Clock
	Cfg                 AuthLifecycleConfig
	NewBootstrapToken   func() string
	NewGrantID          func() string
	NewSessionToken     func() string
	Persist             func() error
}

func (a AuthLifecycle) now() time.Time {
	if a.Now == nil {
		return time.Now().UTC()
	}
	return a.Now().UTC()
}

func (a AuthLifecycle) persist() error {
	if a.Persist != nil {
		return a.Persist()
	}
	return nil
}

func (a *AuthLifecycle) lockMutation() func() {
	if a == nil || a.MutationLock == nil {
		return func() {}
	}
	a.MutationLock.Lock()
	return a.MutationLock.Unlock
}

func (a *AuthLifecycle) CreateBootstrap(agentID, containerID, actorType, actorID string) (auth.BootstrapToken, error) {
	defer a.lockMutation()()
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(containerID) == "" {
		return auth.BootstrapToken{}, errors.New("agent_id and container_id are required")
	}
	now := a.now()
	snapshot := a.Store.Snapshot()
	t := auth.BootstrapToken{
		Token:       a.NewBootstrapToken(),
		AgentID:     agentID,
		ContainerID: containerID,
		CreatedAt:   now,
		ExpiresAt:   now.Add(time.Duration(a.Cfg.BootstrapTokenTTLSeconds) * time.Second),
	}
	a.Store.SaveBootstrap(t)
	if err := a.persist(); err != nil {
		a.Store.Restore(snapshot)
		return auth.BootstrapToken{}, wrapRollbackError(err, a.persist())
	}
	if err := a.auditCritical(ports.AuditEvent{Event: "auth_bootstrap_created", Timestamp: now, ActorType: actorType, ActorID: actorID, AgentID: agentID, Metadata: map[string]string{"container_id": containerID}}); err != nil {
		a.Store.Restore(snapshot)
		return auth.BootstrapToken{}, wrapRollbackError(err, a.persist())
	}
	return t, nil
}

func (a *AuthLifecycle) CompletePairing(token, containerID string) (auth.PairingGrant, error) {
	defer a.lockMutation()()
	if strings.TrimSpace(token) == "" || strings.TrimSpace(containerID) == "" {
		return auth.PairingGrant{}, errors.New("token and container_id are required")
	}
	snapshot := a.Store.Snapshot()
	bt, err := a.Store.ConsumeBootstrap(token, containerID, a.now())
	if err != nil {
		_ = a.Audit.Write(ports.AuditEvent{Event: "auth_pair_denied", Timestamp: a.now(), ActorType: "agent", ActorID: "unknown", Metadata: map[string]string{"reason": err.Error(), "container_id": containerID}})
		return auth.PairingGrant{}, err
	}
	now := a.now()
	g := auth.PairingGrant{
		GrantID:           a.NewGrantID(),
		AgentID:           bt.AgentID,
		ContainerID:       containerID,
		CreatedAt:         now,
		LastUsedAt:        now,
		IdleExpiresAt:     now.Add(time.Duration(a.Cfg.GrantIdleTimeoutMinutes) * time.Minute),
		AbsoluteExpiresAt: now.Add(time.Duration(a.Cfg.GrantAbsoluteMaxMinutes) * time.Minute),
	}
	a.Store.SaveGrant(g)
	if err := a.persist(); err != nil {
		a.Store.Restore(snapshot)
		return auth.PairingGrant{}, wrapRollbackError(err, a.persist())
	}
	if err := a.auditCritical(ports.AuditEvent{Event: "auth_pair_completed", Timestamp: now, ActorType: "agent", ActorID: bt.AgentID, AgentID: bt.AgentID, Metadata: authAuditMetadata(map[string]string{"container_id": containerID, "grant_id": g.GrantID})}); err != nil {
		a.Store.Restore(snapshot)
		return auth.PairingGrant{}, wrapRollbackError(err, a.persist())
	}
	return g, nil
}

func (a *AuthLifecycle) MintSession(grantID string) (auth.SessionToken, error) {
	defer a.lockMutation()()
	g, err := a.Store.GetGrant(grantID)
	if err != nil {
		return auth.SessionToken{}, err
	}
	now := a.now()
	if g.Revoked || !now.Before(g.IdleExpiresAt) || !now.Before(g.AbsoluteExpiresAt) {
		return auth.SessionToken{}, errors.New("grant expired or revoked")
	}
	snapshot := a.Store.Snapshot()
	g.LastUsedAt = now
	g.IdleExpiresAt = now.Add(time.Duration(a.Cfg.GrantIdleTimeoutMinutes) * time.Minute)
	a.Store.UpdateGrant(g)
	st := auth.SessionToken{
		Token:     a.NewSessionToken(),
		GrantID:   g.GrantID,
		AgentID:   g.AgentID,
		CreatedAt: now,
		ExpiresAt: now.Add(time.Duration(a.Cfg.SessionTTLMinutes) * time.Minute),
	}
	a.Store.SaveSession(st)
	if err := a.persist(); err != nil {
		a.Store.Restore(snapshot)
		return auth.SessionToken{}, wrapRollbackError(err, a.persist())
	}
	if err := a.auditCritical(ports.AuditEvent{Event: "auth_session_minted", Timestamp: now, ActorType: "agent", ActorID: g.AgentID, AgentID: g.AgentID, Metadata: authAuditMetadata(map[string]string{"grant_id": g.GrantID})}); err != nil {
		a.Store.Restore(snapshot)
		return auth.SessionToken{}, wrapRollbackError(err, a.persist())
	}
	return st, nil
}

func (a *AuthLifecycle) Revoke(grantID, sessionID, actorType, actorID string) error {
	defer a.lockMutation()()
	if strings.TrimSpace(grantID) == "" && strings.TrimSpace(sessionID) == "" {
		return errors.New("grant_id or session_token is required")
	}
	snapshot := a.Store.Snapshot()
	if strings.TrimSpace(grantID) != "" {
		if err := a.Store.RevokeGrant(grantID); err != nil {
			a.Store.Restore(snapshot)
			return err
		}
	}
	if strings.TrimSpace(sessionID) != "" {
		if err := a.Store.RevokeSession(sessionID); err != nil {
			a.Store.Restore(snapshot)
			return err
		}
	}
	if err := a.persist(); err != nil {
		a.Store.Restore(snapshot)
		return wrapRollbackError(err, a.persist())
	}
	if err := a.auditCritical(ports.AuditEvent{Event: "auth_revoked", Timestamp: a.now(), ActorType: actorType, ActorID: actorID, Metadata: authAuditMetadata(map[string]string{"grant_id": grantID, "session_token": sessionID})}); err != nil {
		a.Store.Restore(snapshot)
		return wrapRollbackError(err, a.persist())
	}
	return nil
}

func (a AuthLifecycle) auditCritical(event ports.AuditEvent) error {
	if a.Audit == nil {
		return nil
	}
	if err := a.Audit.Write(event); err != nil {
		if a.AuditFailureHandler != nil {
			if handled := a.AuditFailureHandler(err); handled != nil {
				return handled
			}
		}
		return ErrAuditUnavailable
	}
	return nil
}

func authAuditMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		switch key {
		case "bootstrap_token", "grant_id", "session_token":
			out[key] = hashedAuditCredentialRef(trimmed)
		default:
			out[key] = trimmed
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func hashedAuditCredentialRef(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	full := hex.EncodeToString(sum[:])
	if len(full) > 16 {
		full = full[:16]
	}
	return "tokhash_" + full
}
