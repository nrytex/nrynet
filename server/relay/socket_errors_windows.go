//go:build windows

package relay

import (
	"errors"
	"syscall"
)

func isExpectedSocketClose(err error) bool {
	return errors.Is(err, syscall.WSAECONNRESET) ||
		errors.Is(err, syscall.WSAECONNABORTED) ||
		errors.Is(err, syscall.ERROR_BROKEN_PIPE) ||
		errors.Is(err, syscall.ERROR_OPERATION_ABORTED)
}
