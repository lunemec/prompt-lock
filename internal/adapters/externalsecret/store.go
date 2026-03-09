package externalsecret

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
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
		return nil, fmt.Errorf("external secret source requires non-empty base URL")
	}

	u, err := url.Parse(rawBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid external secret source base URL %q: %w", baseURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid external secret source base URL %q", baseURL)
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

func (s *Store) GetSecret(name string) (string, error) {
	secretName := strings.TrimSpace(name)
	if secretName == "" {
		return "", fmt.Errorf("secret name is required")
	}

	req, err := http.NewRequest(http.MethodGet, s.secretURL(secretName), nil)
	if err != nil {
		return "", fmt.Errorf("build external secret request: %w", err)
	}

	if s.authTokenEnvVar != "" {
		token := strings.TrimSpace(os.Getenv(s.authTokenEnvVar))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request external secret source: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read external secret response: %w", err)
	}
	trimmedBody := strings.TrimSpace(string(bodyBytes))

	if resp.StatusCode < http.StatusOK || resp.StatusCode > 299 {
		if trimmedBody == "" {
			return "", fmt.Errorf("external secret source returned status %d", resp.StatusCode)
		}
		return "", fmt.Errorf("external secret source returned status %d: %s", resp.StatusCode, trimmedBody)
	}

	if trimmedBody == "" {
		return "", fmt.Errorf("external secret source returned empty secret value")
	}

	if strings.HasPrefix(trimmedBody, "{") {
		var payload struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal([]byte(trimmedBody), &payload); err != nil {
			return "", fmt.Errorf("parse external secret JSON response: %w", err)
		}
		value := strings.TrimSpace(payload.Value)
		if value == "" {
			return "", fmt.Errorf("external secret source returned empty secret value")
		}
		return value, nil
	}

	return trimmedBody, nil
}

func (s *Store) secretURL(secretName string) string {
	return s.baseEndpoint + "/v1/secrets/" + url.PathEscape(secretName)
}
