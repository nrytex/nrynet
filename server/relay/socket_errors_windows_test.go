//go:build windows

package relay

import (
	"fmt"
	"syscall"
	"testing"
)

func TestNormalizeCopyErrorIgnoresWindowsPeerReset(t *testing.T) {
	if err := normalizeCopyError(fmt.Errorf("read: %w", syscall.WSAECONNRESET)); err != nil {
		t.Fatal(err)
	}
}
