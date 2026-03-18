package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

var errWatchBrokerReadyTimeout = errors.New("watch broker readiness timed out")

const (
	defaultWatchBrokerReadyTimeout = 5 * time.Second
	watchBrokerReadyPollInterval   = 100 * time.Millisecond
)

func waitForWatchBrokerReady(conn brokerFlags, operatorToken string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = defaultWatchBrokerReadyTimeout
	}
	deadline := time.Now().Add(timeout)
	var lastErr error

	for {
		broker, err := conn.resolve(brokerRoleOperator)
		if err == nil {
			err = probeWatchBrokerReady(broker.BaseURL, broker.UnixSocket, operatorToken)
			if err == nil {
				return nil
			}
		}
		lastErr = err
		if time.Now().After(deadline) {
			if lastErr == nil {
				lastErr = errWatchBrokerReadyTimeout
			}
			return fmt.Errorf("%w: %v", errWatchBrokerReadyTimeout, lastErr)
		}
		time.Sleep(watchBrokerReadyPollInterval)
	}
}

func probeWatchBrokerReady(baseURL, unixSocket, operatorToken string) error {
	req, err := http.NewRequest(http.MethodGet, buildURL(baseURL, unixSocket, "/v1/requests/pending"), nil)
	if err != nil {
		return err
	}
	if operatorToken != "" {
		req.Header.Set("Authorization", "Bearer "+operatorToken)
	}
	client, err := httpClient(baseURL, unixSocket)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return normalizeBrokerRequestError(err)
	}
	defer resp.Body.Close()
	return nil
}
