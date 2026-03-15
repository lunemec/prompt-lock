package externalstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

const defaultTimeout = 10 * time.Second

type Store struct {
	baseEndpoint    string
	authTokenEnvVar string
	client          *http.Client
}

func New(baseURL, authTokenEnvVarName string, timeoutSeconds int) (*Store, error) {
	rawBaseURL := strings.TrimSpace(baseURL)
	if rawBaseURL == "" {
		return nil, fmt.Errorf("external state store requires non-empty base URL")
	}

	u, err := url.Parse(rawBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid external state store base URL %q: %w", baseURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid external state store base URL %q", baseURL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported external state store URL scheme %q (expected http or https)", u.Scheme)
	}

	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	normalized := *u
	normalized.Path = strings.TrimSuffix(normalized.Path, "/")
	normalized.RawPath = ""
	normalized.RawQuery = ""
	normalized.Fragment = ""
	baseEndpoint := strings.TrimSuffix(normalized.String(), "/")

	return &Store{
		baseEndpoint:    baseEndpoint,
		authTokenEnvVar: strings.TrimSpace(authTokenEnvVarName),
		client:          &http.Client{Timeout: timeout},
	}, nil
}

func (s *Store) SaveRequest(req domain.LeaseRequest) error {
	if strings.TrimSpace(req.ID) == "" {
		return errors.New("request id is required")
	}
	return s.send(http.MethodPut, s.requestURL(req.ID), req, nil, "")
}

func (s *Store) GetRequest(id string) (domain.LeaseRequest, error) {
	reqID := strings.TrimSpace(id)
	if reqID == "" {
		return domain.LeaseRequest{}, errors.New("request id is required")
	}
	var req domain.LeaseRequest
	if err := s.send(http.MethodGet, s.requestURL(reqID), nil, &req, "request not found"); err != nil {
		return domain.LeaseRequest{}, err
	}
	return req, nil
}

func (s *Store) UpdateRequest(req domain.LeaseRequest) error {
	if strings.TrimSpace(req.ID) == "" {
		return errors.New("request id is required")
	}
	return s.send(http.MethodPut, s.requestURL(req.ID), req, nil, "")
}

func (s *Store) DeleteRequest(id string) error {
	reqID := strings.TrimSpace(id)
	if reqID == "" {
		return errors.New("request id is required")
	}
	return s.send(http.MethodDelete, s.requestURL(reqID), nil, nil, "request not found")
}

func (s *Store) ListPendingRequests() ([]domain.LeaseRequest, error) {
	var out struct {
		Pending []domain.LeaseRequest `json:"pending"`
	}
	if err := s.send(http.MethodGet, s.pendingRequestsURL(), nil, &out, ""); err != nil {
		return nil, err
	}
	if out.Pending == nil {
		return []domain.LeaseRequest{}, nil
	}
	return out.Pending, nil
}

func (s *Store) SaveLease(lease domain.Lease) error {
	if strings.TrimSpace(lease.Token) == "" {
		return errors.New("lease token is required")
	}
	return s.send(http.MethodPut, s.leaseURL(lease.Token), lease, nil, "")
}

func (s *Store) DeleteLease(token string) error {
	leaseToken := strings.TrimSpace(token)
	if leaseToken == "" {
		return errors.New("lease token is required")
	}
	return s.send(http.MethodDelete, s.leaseURL(leaseToken), nil, nil, "lease not found")
}

func (s *Store) GetLease(token string) (domain.Lease, error) {
	leaseToken := strings.TrimSpace(token)
	if leaseToken == "" {
		return domain.Lease{}, errors.New("lease token is required")
	}
	var lease domain.Lease
	if err := s.send(http.MethodGet, s.leaseURL(leaseToken), nil, &lease, "lease not found"); err != nil {
		return domain.Lease{}, err
	}
	return lease, nil
}

func (s *Store) GetLeaseByRequestID(requestID string) (domain.Lease, error) {
	reqID := strings.TrimSpace(requestID)
	if reqID == "" {
		return domain.Lease{}, errors.New("request id is required")
	}
	var lease domain.Lease
	if err := s.send(http.MethodGet, s.leaseByRequestURL(reqID), nil, &lease, "lease not found for request"); err != nil {
		return domain.Lease{}, err
	}
	return lease, nil
}

func (s *Store) send(method, endpoint string, payload any, out any, notFoundErr string) error {
	body, err := s.encodePayload(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return ports.WrapStoreUnavailable(fmt.Errorf("build external state request: %w", err))
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := s.applyAuth(req); err != nil {
		return err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return ports.WrapStoreUnavailable(fmt.Errorf("request external state backend: %w", err))
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ports.WrapStoreUnavailable(fmt.Errorf("read external state response: %w", err))
	}
	bodyText := strings.TrimSpace(string(rawBody))

	if resp.StatusCode == http.StatusNotFound && notFoundErr != "" {
		return errors.New(notFoundErr)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode > 299 {
		if bodyText == "" {
			bodyText = http.StatusText(resp.StatusCode)
		}
		return ports.WrapStoreUnavailable(fmt.Errorf("external state backend returned status %d: %s", resp.StatusCode, bodyText))
	}

	if out == nil {
		return nil
	}
	if bodyText == "" {
		return ports.WrapStoreUnavailable(errors.New("external state backend returned empty response body"))
	}
	if err := json.Unmarshal(rawBody, out); err != nil {
		return ports.WrapStoreUnavailable(fmt.Errorf("parse external state response: %w", err))
	}
	return nil
}

func (s *Store) applyAuth(req *http.Request) error {
	if s.authTokenEnvVar == "" {
		return nil
	}
	token := strings.TrimSpace(os.Getenv(s.authTokenEnvVar))
	if token == "" {
		return ports.WrapStoreUnavailable(fmt.Errorf("external state auth token env %s is empty", s.authTokenEnvVar))
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

func (s *Store) encodePayload(payload any) (io.Reader, error) {
	if payload == nil {
		return nil, nil
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

func (s *Store) requestURL(id string) string {
	return s.baseEndpoint + "/v1/state/requests/" + url.PathEscape(id)
}

func (s *Store) pendingRequestsURL() string {
	return s.baseEndpoint + "/v1/state/requests/pending"
}

func (s *Store) leaseURL(token string) string {
	return s.baseEndpoint + "/v1/state/leases/" + url.PathEscape(token)
}

func (s *Store) leaseByRequestURL(requestID string) string {
	return s.baseEndpoint + "/v1/state/leases/by-request/" + url.PathEscape(requestID)
}
