package app

import (
	"fmt"
	"math"
	"time"
)

type RequestThrottleReason string

const (
	RequestThrottleReasonPendingCap RequestThrottleReason = "pending_cap"
	RequestThrottleReasonCooldown   RequestThrottleReason = "cooldown"
)

type RequestThrottleError struct {
	Reason     RequestThrottleReason
	RetryAfter time.Duration
}

func NewRequestThrottleError(reason RequestThrottleReason, retryAfter time.Duration) *RequestThrottleError {
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return &RequestThrottleError{
		Reason:     reason,
		RetryAfter: retryAfter,
	}
}

func (e *RequestThrottleError) RetryAfterSeconds() int {
	if e == nil {
		return 1
	}
	seconds := int(math.Ceil(e.RetryAfter.Seconds()))
	if seconds < 1 {
		return 1
	}
	return seconds
}

func (e *RequestThrottleError) Error() string {
	message := "request throttled"
	switch e.Reason {
	case RequestThrottleReasonPendingCap:
		message = "pending request cap reached"
	case RequestThrottleReasonCooldown:
		message = "equivalent request cooldown active"
	}
	return fmt.Sprintf("%s; retry_after_seconds=%d", message, e.RetryAfterSeconds())
}
