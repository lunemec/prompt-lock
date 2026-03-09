package main

import (
	"errors"
	"net/http"
)

var (
	ErrBadRequest         = errors.New("bad_request")
	ErrMethodNotAllowed   = errors.New("method_not_allowed")
	ErrRateLimited        = errors.New("rate_limited")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrNotFound           = errors.New("not_found")
	ErrServiceUnavailable = errors.New("service_unavailable")
	ErrInternal           = errors.New("internal_error")
)

func writeMappedError(w http.ResponseWriter, kind error, msg string) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(kind, ErrBadRequest):
		status = http.StatusBadRequest
	case errors.Is(kind, ErrMethodNotAllowed):
		status = http.StatusMethodNotAllowed
	case errors.Is(kind, ErrRateLimited):
		status = http.StatusTooManyRequests
	case errors.Is(kind, ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(kind, ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(kind, ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(kind, ErrServiceUnavailable):
		status = http.StatusServiceUnavailable
	case errors.Is(kind, ErrInternal):
		status = http.StatusInternalServerError
	}
	http.Error(w, msg, status)
}
