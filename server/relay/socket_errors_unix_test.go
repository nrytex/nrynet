//go:build !windows

package relay

import (
	"fmt"
	"syscall"
	"testing"
)

func TestNormalizeCopyErrorIgnoresUnixPeerReset(t *testing.T) {
	if err := normalizeCopyError(fmt.Errorf("read: %w", syscall.ECONNRESET)); err != nil {
		t.Fatal(err)
	}
}
