package agent

import (
	"context"
	"errors"
	"testing"
)

func TestOpenDataWithRetryDoesNotDuplicateSingleOpen(t *testing.T) {
	control := &singleOpenControl{}
	_, err := openDataWithRetry(context.Background(), control, "single-1")
	if err == nil {
		t.Fatal("expected single open to fail")
	}
	if control.calls != 1 {
		t.Fatalf("single open calls=%d, want 1", control.calls)
	}
}

type singleOpenControl struct {
	calls int
}

func (*singleOpenControl) readJSON(any) error   { return nil }
func (*singleOpenControl) writeJSON(any) error  { return nil }
func (*singleOpenControl) close() error         { return nil }
func (*singleOpenControl) singleDataOpen() bool { return true }

func (c *singleOpenControl) openData(context.Context, string) (dataConn, error) {
	c.calls++
	return nil, errors.New("single open failed")
}
