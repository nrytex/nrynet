//go:build windows

package agent

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

func isRetryableLocalDialError(err error) bool {
	return errors.Is(err, windows.WSAECONNREFUSED) ||
		errors.Is(err, windows.WSAETIMEDOUT) ||
		errors.Is(err, windows.WSAEHOSTUNREACH) ||
		errors.Is(err, windows.WSAENETUNREACH) ||
		errors.Is(err, syscall.WSAECONNABORTED)
}
