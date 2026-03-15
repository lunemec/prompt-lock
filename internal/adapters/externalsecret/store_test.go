package externalsecret

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGetSecretSuccessJSON(t *testing.T) {
	secretName := "db/prod token"
	expectedPath := "/v1/secrets/" + url.PathEscape(secretName)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET method, got %s", r.Method)
		}
		if r.URL.EscapedPath() != expectedPath {
			t.Fatalf("unexpected request path: got %q want %q", r.URL.EscapedPath(), expectedPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":"  top-secret  "}`))
	}))
	defer srv.Close()

	s, err := New(srv.URL, "", 5)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	v, err := s.GetSecret(secretName)
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if v != "  top-secret  " {
		t.Fatalf("unexpected secret value: %q", v)
	}
}

func TestGetSecretSuccessPlaintext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("\n  plain-secret  \n"))
	}))
	defer srv.Close()

	s, err := New(srv.URL, "", 5)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	v, err := s.GetSecret("github_token")
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if v != "\n  plain-secret  \n" {
		t.Fatalf("unexpected secret value: %q", v)
	}
}

func TestGetSecretMissingSecret404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("  missing secret  \n"))
	}))
	defer srv.Close()

	s, err := New(srv.URL, "", 5)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	_, err = s.GetSecret("missing")
	if err == nil {
		t.Fatalf("expected missing secret error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected status code in error, got %q", err)
	}
	if !strings.Contains(err.Error(), "missing secret") {
		t.Fatalf("expected response body in error, got %q", err)
	}
}

func TestGetSecretAuthHeaderWiring(t *testing.T) {
	const tokenEnv = "PROMPTLOCK_EXTERNAL_SECRET_TOKEN"
	t.Setenv(tokenEnv, "token-123")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	s, err := New(srv.URL, tokenEnv, 5)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	v, err := s.GetSecret("auth_test")
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if v != "ok" {
		t.Fatalf("unexpected secret value: %q", v)
	}
}

func TestGetSecretEmptyValue(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty json value", body: `{"value":"   "}`},
		{name: "empty plaintext", body: "   \n\t"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			s, err := New(srv.URL, "", 5)
			if err != nil {
				t.Fatalf("new store: %v", err)
			}

			_, err = s.GetSecret("empty")
			if err == nil {
				t.Fatalf("expected empty secret error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "empty") {
				t.Fatalf("expected empty value error, got %q", err)
			}
		})
	}
}

func TestNewInvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
	}{
		{name: "empty base URL", baseURL: ""},
		{name: "whitespace base URL", baseURL: "   "},
		{name: "invalid URL", baseURL: "://bad"},
		{name: "URL without scheme", baseURL: "example.com"},
		{name: "unsupported scheme", baseURL: "ftp://example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.baseURL, "TOKEN_ENV", 10); err == nil {
				t.Fatalf("expected error for base URL %q", tc.baseURL)
			}
		})
	}
}

func TestGetSecretEmptyConfiguredAuthTokenEnvFailsClosed(t *testing.T) {
	const tokenEnv = "PROMPTLOCK_EXTERNAL_SECRET_TOKEN"
	t.Setenv(tokenEnv, "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("request should not be sent when auth token env is empty")
	}))
	defer srv.Close()

	s, err := New(srv.URL, tokenEnv, 5)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if _, err := s.GetSecret("auth_test"); err == nil {
		t.Fatalf("expected empty configured auth token env to fail")
	}
}

func TestNewTimeoutConfig(t *testing.T) {
	s, err := New("http://127.0.0.1:8080", "", 17)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if s.client.Timeout != 17*time.Second {
		t.Fatalf("unexpected timeout: got %s want %s", s.client.Timeout, 17*time.Second)
	}

	for _, timeoutSeconds := range []int{0, -1} {
		s, err := New("http://127.0.0.1:8080", "", timeoutSeconds)
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		if s.client.Timeout != defaultTimeout {
			t.Fatalf("unexpected default timeout: got %s want %s", s.client.Timeout, defaultTimeout)
		}
	}
}
