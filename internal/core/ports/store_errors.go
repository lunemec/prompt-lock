package ports

import (
	"errors"
	"fmt"
)

var ErrStoreUnavailable = errors.New("store_unavailable")

func WrapStoreUnavailable(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
}
