//go:build windows

package agent

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

func expectedSocketCloseErrors() []error {
	return []error{syscall.WSAECONNRESET, syscall.WSAECONNABORTED, syscall.ERROR_BROKEN_PIPE}
}
