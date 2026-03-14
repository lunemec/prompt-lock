package main

import (
	"errors"

	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/core/ports"
)

const stateStoreUnavailableMessage = "state backend unavailable; retry when backend connectivity is restored"
const secretBackendUnavailableMessage = "secret backend unavailable; retry when backend connectivity is restored"

func stateStoreReadError(err error) (error, string) {
	if errors.Is(err, ports.ErrStoreUnavailable) {
		return ErrServiceUnavailable, stateStoreUnavailableMessage
	}
	if errors.Is(err, app.ErrRequestNotOwned) || errors.Is(err, app.ErrLeaseNotOwned) {
		return ErrForbidden, err.Error()
	}
	return ErrNotFound, err.Error()
}

func stateStoreMutationError(err error) (error, string) {
	if errors.Is(err, ports.ErrStoreUnavailable) {
		return ErrServiceUnavailable, stateStoreUnavailableMessage
	}
	return ErrBadRequest, err.Error()
}

func stateStoreCancelMutationError(err error) (error, string) {
	if errors.Is(err, ports.ErrStoreUnavailable) {
		return ErrServiceUnavailable, stateStoreUnavailableMessage
	}
	if errors.Is(err, app.ErrRequestNotOwned) {
		return ErrForbidden, err.Error()
	}
	return ErrBadRequest, err.Error()
}

func stateStoreAccessError(err error) (error, string) {
	if errors.Is(err, app.ErrSecretBackendUnavailable) {
		return ErrServiceUnavailable, secretBackendUnavailableMessage
	}
	if errors.Is(err, ports.ErrStoreUnavailable) {
		return ErrServiceUnavailable, stateStoreUnavailableMessage
	}
	if errors.Is(err, app.ErrRequestNotOwned) || errors.Is(err, app.ErrLeaseNotOwned) {
		return ErrForbidden, err.Error()
	}
	return ErrForbidden, err.Error()
}

func stateStoreListError(err error) (error, string) {
	if errors.Is(err, ports.ErrStoreUnavailable) {
		return ErrServiceUnavailable, stateStoreUnavailableMessage
	}
	return ErrInternal, err.Error()
}
