package agent

import (
	"context"
	"fmt"
	"net"
	"time"
)

const (
	localDialAttempts       = 16
	localDialInitialBackoff = 100 * time.Millisecond
	localDialMaxBackoff     = time.Second
)

func (a *Agent) dialLocalService(ctx context.Context, address string) (net.Conn, error) {
	if _, _, err := net.SplitHostPort(address); err != nil {
		return nil, fmt.Errorf("invalid local service address: %w", err)
	}
	return dialLocalWithRetry(ctx, address)
}

func dialLocalWithRetry(ctx context.Context, address string) (net.Conn, error) {
	var lastErr error
	for attempt := 0; attempt < localDialAttempts; attempt++ {
		conn, err := dialTCP(ctx, address)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if ctx.Err() != nil || !isRetryableLocalDialError(err) || attempt == localDialAttempts-1 {
			return nil, lastErr
		}
		if err := waitLocalDialBackoff(ctx, attempt); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func dialTCP(ctx context.Context, address string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, "tcp", address)
}

func waitLocalDialBackoff(ctx context.Context, attempt int) error {
	delay := localDialInitialBackoff << attempt
	if delay > localDialMaxBackoff {
		delay = localDialMaxBackoff
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
