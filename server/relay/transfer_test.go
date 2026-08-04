package relay

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
)

func TestNormalizeCopyErrorIgnoresExpectedConnectionClosures(t *testing.T) {
	for _, err := range []error{nil, io.EOF, io.ErrClosedPipe, net.ErrClosed, context.Canceled} {
		if got := normalizeCopyError(err); got != nil {
			t.Fatalf("normalizeCopyError(%v)=%v", err, got)
		}
	}
}

func TestNormalizeCopyErrorPreservesUnexpectedFailures(t *testing.T) {
	want := errors.New("unexpected relay failure")
	if got := normalizeCopyError(want); !errors.Is(got, want) {
		t.Fatalf("normalizeCopyError()=%v want=%v", got, want)
	}
}
