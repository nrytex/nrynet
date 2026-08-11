//go:build !windows

package agent

import (
	"errors"
	"syscall"
)

func isExpectedSocketClose(err error) bool {
	return errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.EPIPE)
}

func expectedSocketCloseErrors() []error {
	return []error{syscall.ECONNRESET, syscall.ECONNABORTED, syscall.EPIPE}
}
