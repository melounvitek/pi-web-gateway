//go:build !linux && !darwin

package update

import (
	"context"
	"errors"
)

func acquireCheckoutLock(context.Context, string) (func(), error) {
	return nil, errors.New("self-update checkout locking is unsupported on this platform")
}
