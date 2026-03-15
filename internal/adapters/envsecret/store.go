package envsecret

import (
	"fmt"
	"os"
	"strings"
)

type Store struct {
	Prefix string
}

func New(prefix string) *Store {
	p := strings.TrimSpace(prefix)
	if p == "" {
		p = "PROMPTLOCK_SECRET_"
	}
	return &Store{Prefix: p}
}

func (s *Store) GetSecret(name string) (string, error) {
	key := s.Prefix + strings.ToUpper(strings.TrimSpace(name))
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return "", fmt.Errorf("secret %q not found in env source (%s)", name, key)
	}
	return v, nil
}
