package app

import (
	"fmt"
	"strings"

	"github.com/lunemec/promptlock/internal/core/ports"
)

func rollbackCreatedRequest(store ports.RequestStore, requestID string) error {
	return store.DeleteRequest(requestID)
}

func rollbackCreatedLease(store ports.LeaseStore, leaseToken string) error {
	return store.DeleteLease(leaseToken)
}

func (s Service) rollbackRequestLeaseMutation(rollbackFns ...func() error) error {
	errs := make([]error, 0, len(rollbackFns)+1)
	for _, rollbackFn := range rollbackFns {
		if rollbackFn == nil {
			continue
		}
		if err := rollbackFn(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.RequestLeaseStateCommitter != nil {
		if err := s.commitRequestLeaseState(); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrors(errs...)
}

func joinErrors(errs ...error) error {
	msgs := make([]string, 0, len(errs))
	for _, err := range errs {
		if err == nil {
			continue
		}
		msgs = append(msgs, err.Error())
	}
	if len(msgs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(msgs, "; "))
}

func wrapRollbackError(cause error, rollbackErrs ...error) error {
	msgs := make([]string, 0, len(rollbackErrs))
	for _, rollbackErr := range rollbackErrs {
		if rollbackErr == nil {
			continue
		}
		msgs = append(msgs, rollbackErr.Error())
	}
	if len(msgs) == 0 {
		return cause
	}
	return fmt.Errorf("%w (rollback failed: %s)", cause, strings.Join(msgs, "; "))
}
