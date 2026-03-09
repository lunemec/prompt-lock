package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type leaseRequest struct {
	RequestID  string    `json:"request_id"`
	Status     string    `json:"status"`
	AgentID    string    `json:"agent_id"`
	TaskID     string    `json:"task_id"`
	Reason     string    `json:"reason"`
	TTLMinutes int       `json:"ttl_minutes"`
	Secrets    []string  `json:"secrets"`
	CreatedAt  time.Time `json:"created_at"`
}

type lease struct {
	LeaseToken string    `json:"lease_token"`
	RequestID  string    `json:"request_id"`
	AgentID    string    `json:"agent_id"`
	TaskID     string    `json:"task_id"`
	Secrets    []string  `json:"secrets"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type state struct {
	mu       sync.Mutex
	Requests map[string]leaseRequest
	Leases   map[string]lease
	Audit    []map[string]any
}

var (
	host      = envDefault("PROMPTLOCK_MOCK_HOST", "127.0.0.1")
	port      = envDefault("PROMPTLOCK_MOCK_PORT", "8765")
	maxTTLMin = 60
	secrets   = map[string]string{
		"github_token":   "DEMO_GITHUB_TOKEN_REPLACE_ME",
		"npm_token":      "DEMO_NPM_TOKEN_REPLACE_ME",
		"openai_api_key": "DEMO_OPENAI_KEY_REPLACE_ME",
	}
	st = state{
		Requests: map[string]leaseRequest{},
		Leases:   map[string]lease{},
		Audit:    []map[string]any{},
	}
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/meta/capabilities", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, map[string]any{"mock": true, "allow_plaintext_secret_return": true, "auth_enabled": false})
	})
	mux.HandleFunc("/v1/leases/request", handleLeaseRequest)
	mux.HandleFunc("/v1/leases/access", handleLeaseAccess)
	mux.HandleFunc("/v1/leases/", handleLeaseDecision)

	addr := host + ":" + port
	log.Printf("Mock PromptLock broker listening on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func handleLeaseRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		AgentID    string   `json:"agent_id"`
		TaskID     string   `json:"task_id"`
		Reason     string   `json:"reason"`
		TTLMinutes int      `json:"ttl_minutes"`
		Secrets    []string `json:"secrets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.TTLMinutes <= 0 || len(req.Secrets) == 0 {
		http.Error(w, "ttl_minutes>0 and secrets required", http.StatusBadRequest)
		return
	}
	id := "req_" + randomHex(8)
	st.mu.Lock()
	st.Requests[id] = leaseRequest{
		RequestID:  id,
		Status:     "pending",
		AgentID:    defaultString(req.AgentID, "unknown"),
		TaskID:     defaultString(req.TaskID, "unknown"),
		Reason:     req.Reason,
		TTLMinutes: req.TTLMinutes,
		Secrets:    append([]string{}, req.Secrets...),
		CreatedAt:  time.Now().UTC(),
	}
	st.Audit = append(st.Audit, map[string]any{"event": "request_created", "request_id": id, "at": time.Now().UTC().Format(time.RFC3339Nano)})
	st.mu.Unlock()
	writeJSON(w, map[string]any{"request_id": id, "status": "pending"})
}

func handleLeaseDecision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/leases/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	requestID, action := parts[0], parts[1]

	st.mu.Lock()
	defer st.mu.Unlock()
	req, ok := st.Requests[requestID]
	if !ok {
		http.Error(w, "request not found", http.StatusNotFound)
		return
	}
	if action == "deny" {
		req.Status = "denied"
		st.Requests[requestID] = req
		st.Audit = append(st.Audit, map[string]any{"event": "request_denied", "request_id": requestID, "at": time.Now().UTC().Format(time.RFC3339Nano)})
		writeJSON(w, map[string]any{"status": "denied"})
		return
	}
	if action != "approve" {
		http.NotFound(w, r)
		return
	}
	if req.Status != "pending" {
		http.Error(w, fmt.Sprintf("request already %s", req.Status), http.StatusBadRequest)
		return
	}
	var approve struct {
		TTLMinutes int `json:"ttl_minutes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&approve)
	ttl := req.TTLMinutes
	if approve.TTLMinutes > 0 {
		ttl = approve.TTLMinutes
	}
	if ttl > maxTTLMin {
		ttl = maxTTLMin
	}
	leaseTok := "lease_" + randomHex(12)
	expiresAt := time.Now().UTC().Add(time.Duration(ttl) * time.Minute)
	req.Status = "approved"
	st.Requests[requestID] = req
	st.Leases[leaseTok] = lease{
		LeaseToken: leaseTok,
		RequestID:  requestID,
		AgentID:    req.AgentID,
		TaskID:     req.TaskID,
		Secrets:    append([]string{}, req.Secrets...),
		ExpiresAt:  expiresAt,
	}
	st.Audit = append(st.Audit, map[string]any{"event": "request_approved", "request_id": requestID, "lease_token": leaseTok, "at": time.Now().UTC().Format(time.RFC3339Nano)})
	writeJSON(w, map[string]any{"status": "approved", "lease_token": leaseTok, "expires_at": expiresAt, "secrets": req.Secrets})
}

func handleLeaseAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		LeaseToken string `json:"lease_token"`
		Secret     string `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	l, ok := st.Leases[req.LeaseToken]
	if !ok {
		http.Error(w, "invalid lease", http.StatusForbidden)
		return
	}
	if !time.Now().UTC().Before(l.ExpiresAt) {
		http.Error(w, "lease expired", http.StatusForbidden)
		return
	}
	if !contains(l.Secrets, req.Secret) {
		http.Error(w, "secret not allowed in lease", http.StatusForbidden)
		return
	}
	v, ok := secrets[req.Secret]
	if !ok {
		http.Error(w, "unknown secret", http.StatusNotFound)
		return
	}
	st.Audit = append(st.Audit, map[string]any{"event": "secret_access", "lease_token": req.LeaseToken, "secret": req.Secret, "at": time.Now().UTC().Format(time.RFC3339Nano)})
	writeJSON(w, map[string]any{"secret": req.Secret, "value": v})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func defaultString(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

func contains(values []string, needle string) bool {
	for _, v := range values {
		if v == needle {
			return true
		}
	}
	return false
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func envDefault(k, d string) string {
	if v := os.Getenv(k); strings.TrimSpace(v) != "" {
		return v
	}
	return d
}
