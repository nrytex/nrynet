package agent

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/nrytex/nrynet/internal/protocol"
)

type Agent struct {
	options             Options
	logger              *slog.Logger
	udp                 *udpRelay
	visitorMu           sync.Mutex
	visitorSlots        chan struct{}
	visitorSessionMu    sync.Mutex
	visitorSessionSlots chan struct{}
	streamWorkerMu      sync.Mutex
	streamWorkerSlots   chan struct{}
	relayMu             sync.Mutex
	relaySlots          chan struct{}
	controlMu           sync.Mutex
	webSocketFallback   bool
	sessionEstablished  bool
}

type controlConn interface {
	readJSON(value any) error
	writeJSON(value any) error
	close() error
	openData(context.Context, string) (dataConn, error)
}

type dataConn interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
}

func New(options Options, logger *slog.Logger) (*Agent, error) {
	if logger == nil {
		logger = slog.Default()
	}
	options = normalizeOptions(options)
	if err := options.Validate(); err != nil {
		return nil, err
	}
	return &Agent{options: options, logger: logger, udp: newUDPRelay(2 * time.Minute)}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	backoff := a.reconnectMin()
	for ctx.Err() == nil {
		err := a.runWorker("agent session", func() error { return a.runSession(ctx) })
		if ctx.Err() != nil {
			return nil
		}
		sessionEstablished := a.consumeSessionEstablished()
		if sessionEstablished {
			backoff = a.options.ReconnectMin
		}
		a.logger.Warn("agent control session ended", "error", err, "retry_in", backoff)
		sleep(ctx, backoff)
		if sessionEstablished {
			continue
		}
		backoff = nextBackoff(backoff, a.options.ReconnectMax)
	}
	return nil
}

func (a *Agent) reconnectMin() time.Duration {
	if a.options.ReconnectMin > 0 {
		return a.options.ReconnectMin
	}
	return time.Second
}

func (a *Agent) runSession(ctx context.Context) (sessionErr error) {
	a.clearSessionEstablished()
	conn, err := a.dialControl(ctx)
	if err != nil {
		a.notifySessionEnded(err)
		return err
	}
	defer conn.close()
	defer func() {
		if sessionErr != nil && ctx.Err() == nil {
			a.markWebSocketFallback(conn, sessionErr)
		}
	}()
	if err := a.sendHello(conn); err != nil {
		a.notifySessionEnded(err)
		return err
	}
	a.notifySessionStarted()
	a.markSessionEstablished()
	errCh := make(chan error, 2)
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		errCh <- a.runWorker("heartbeat", func() error { return a.heartbeat(sessionCtx, conn) })
	}()
	go func() {
		defer workers.Done()
		errCh <- a.runWorker("control read loop", func() error { return a.readLoop(sessionCtx, conn) })
	}()
	sessionErr = <-errCh
	cancel()
	_ = conn.close()
	workers.Wait()
	a.notifySessionEnded(sessionErr)
	return sessionErr
}

func (a *Agent) readLoop(ctx context.Context, conn controlConn) error {
	for ctx.Err() == nil {
		var message protocol.ControlMessage
		if err := conn.readJSON(&message); err != nil {
			return fmt.Errorf("read control message: %w", err)
		}
		if err := a.handleControlMessage(ctx, conn, message); err != nil {
			a.logger.Warn("control message failed", "type", message.Type, "error", err)
		}
	}
	return nil
}

func (a *Agent) sendHello(conn controlConn) error {
	payload := protocol.HelloPayload{
		Name:     a.options.Config.Name,
		DeviceID: a.options.Config.DeviceID,
		OS:       runtime.GOOS,
		Version:  a.options.Version,
	}
	message, err := protocol.NewMessage(protocol.TypeHello, "", "", payload)
	if err != nil {
		return err
	}
	return conn.writeJSON(message)
}
