package config

import "fmt"

type IntentMap map[string][]string

func (c Config) ResolveIntent(intent string) ([]string, error) {
	if c.Intents == nil {
		return nil, fmt.Errorf("intent map not configured")
	}
	secrets, ok := c.Intents[intent]
	if !ok || len(secrets) == 0 {
		return nil, fmt.Errorf("unknown or empty intent: %s", intent)
	}
	return append([]string{}, secrets...), nil
}
