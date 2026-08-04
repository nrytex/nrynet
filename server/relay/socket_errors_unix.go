//go:build !windows

package relay

import (
	"errors"
	"syscall"
)

func isExpectedSocketClose(err error) bool {
	return errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.EPIPE)
}
