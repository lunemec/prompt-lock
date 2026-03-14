package app

import "errors"

var ErrSecretBackendUnavailable = errors.New("secret backend unavailable")

var ErrRequestNotOwned = errors.New("request not owned by agent")

var ErrLeaseNotOwned = errors.New("lease not owned by agent")
