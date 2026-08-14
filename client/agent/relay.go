package agent

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nrytex/nrynet/internal/protocol"
)

const (
	openConnectionSetupTimeout = 10 * time.Second
	dataChannelOpenAttempts    = 3
	quicDataChannelTimeout     = 5 * time.Second
)

var relayCopyBufferPool = sync.Pool{
	New: func() any { return make([]byte, 32*1024) },
}

func (a *Agent) handleOpenConnection(ctx context.Context, conn controlConn, message protocol.ControlMessage) {
	if err := a.openAndRelay(ctx, conn, message); err != nil {
		a.logger.Warn("connection relay failed", "request_id", message.RequestID, "error", err)
		a.reportConnectionFailure(ctx, conn, message, err)
		return
	}
}

func (a *Agent) openAndRelay(ctx context.Context, conn controlConn, message protocol.ControlMessage) error {
	payload, err := protocol.DecodePayload[protocol.OpenConnectionPayload](message)
	if err != nil {
		return err
	}
	setupCtx, cancel := context.WithTimeout(ctx, openConnectionSetupTimeout)
	defer cancel()
	localAddress := net.JoinHostPort(payload.LocalHost, strconv.Itoa(payload.LocalPort))
	localConn, err := a.dialLocalService(setupCtx, localAddress)
	if err != nil {
		return normalizeSetupError("dial local service", err)
	}
	defer localConn.Close()
	dataConn, err := openDataWithRetry(setupCtx, conn, message.RequestID)
	if err != nil {
		return normalizeSetupError("dial data channel", err)
	}
	defer dataConn.Close()
	if a.options.Config.Transport != "quic" || needsDataHandshake(dataConn) {
		err = writeHandshake(dataConn, a.options.Config.Token, a.options.Config.DeviceID, message.RequestID)
	}
	if err != nil {
		return normalizeSetupError("write data handshake", err)
	}
	return a.relay(ctx, message.TunnelID, dataConn, localConn)
}

type dataChannel struct {
	dataConn
	needsHandshake bool
}

func (c *dataChannel) needsDataHandshake() bool { return c.needsHandshake }

func needsDataHandshake(data dataConn) bool {
	channel, ok := data.(*dataChannel)
	return ok && channel.needsDataHandshake()
}

func openDataWithRetry(ctx context.Context, conn controlConn, requestID string) (dataConn, error) {
	if single, ok := conn.(interface{ singleDataOpen() bool }); ok && single.singleDataOpen() {
		// A failed QUIC stream open can already have reached the server. Do not
		// create duplicate streams with the same request ID; QUIC's own data
		// path performs the TCP fallback within this single attempt.
		return conn.openData(ctx, requestID)
	}
	var lastErr error
	for attempt := 0; attempt < dataChannelOpenAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		data, err := conn.openData(attemptCtx, requestID)
		cancel()
		if err == nil {
			return data, nil
		}
		lastErr = err
		if ctx.Err() != nil || attempt == dataChannelOpenAttempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	return nil, lastErr
}

func openQUICDataWithFallback(
	ctx context.Context,
	requestID string,
	quicOpen func(context.Context, string) (dataConn, error),
	legacyOpen func(context.Context) (dataConn, error),
) (dataConn, error, bool) {
	quicCtx, cancel := context.WithTimeout(ctx, quicDataChannelTimeout)
	data, quicErr := quicOpen(quicCtx, requestID)
	cancel()
	if quicErr == nil {
		return data, nil, false
	}
	if ctx.Err() != nil {
		return nil, quicErr, false
	}
	data, legacyErr := legacyOpen(ctx)
	if legacyErr == nil {
		return data, nil, true
	}
	return nil, fmt.Errorf("QUIC data channel: %w; TCP relay fallback: %w", quicErr, legacyErr), true
}

func (a *Agent) dialLegacyData(ctx context.Context) (dataConn, error) {
	if !strings.HasPrefix(strings.ToLower(a.options.Config.ServerURL), "wss://") {
		return dialTCP(ctx, a.options.Config.DataAddress)
	}
	tlsConfig, err := secureClientTLS(tlsServerName(a.options.Config.DataAddress), a.options.Config)
	if err != nil {
		return nil, err
	}
	dialer := tls.Dialer{Config: tlsConfig}
	return dialer.DialContext(ctx, "tcp", a.options.Config.DataAddress)
}

func writeHandshake(writer io.Writer, token, deviceID, requestID string) error {
	buffered := bufio.NewWriter(writer)
	handshake := protocol.DataHandshake{Token: token, DeviceID: deviceID, RequestID: requestID}
	if err := json.NewEncoder(buffered).Encode(handshake); err != nil {
		return fmt.Errorf("write data handshake: %w", err)
	}
	return buffered.Flush()
}

func (a *Agent) relay(ctx context.Context, tunnelID string, left dataConn, right net.Conn) error {
	defer a.flushTransfer(tunnelID)
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = left.Close()
			_ = right.Close()
		case <-stop:
		}
	}()
	defer close(stop)
	errCh := make(chan error, 2)
	go func() {
		errCh <- a.runWorker("relay upload", func() error {
			return copyBytes(left, right, func(n int64) {
				a.notifyTransfer(tunnelID, DirectionUpload, n)
			})
		})
	}()
	go func() {
		errCh <- a.runWorker("relay download", func() error {
			return copyBytes(right, left, func(n int64) {
				a.notifyTransfer(tunnelID, DirectionDownload, n)
			})
		})
	}()
	firstErr := normalizeCopyError(<-errCh)
	_ = left.Close()
	_ = right.Close()
	secondErr := normalizeCopyError(<-errCh)
	if firstErr != nil {
		return firstErr
	}
	if secondErr != nil {
		a.logger.Debug("relay peer closed", "error", secondErr)
	}
	return nil
}

func copyBytes(dst io.Writer, src io.Reader, observe func(int64)) error {
	buffer := relayCopyBufferPool.Get().([]byte)
	defer relayCopyBufferPool.Put(buffer)
	writer := &transferWriter{writer: dst, observe: observe, lastReport: time.Now()}
	_, err := io.CopyBuffer(writer, src, buffer)
	writer.flush()
	return err
}

type transferWriter struct {
	writer     io.Writer
	observe    func(int64)
	pending    int64
	lastReport time.Time
}

func (w *transferWriter) Write(data []byte) (int, error) {
	written, err := w.writer.Write(data)
	w.pending += int64(written)
	if time.Since(w.lastReport) >= transferReportInterval {
		w.flush()
	}
	return written, err
}

func (w *transferWriter) flush() {
	if w.pending == 0 {
		return
	}
	w.observe(w.pending)
	w.pending = 0
	w.lastReport = time.Now()
}

func normalizeCopyError(err error) error {
	if err == nil || err == io.EOF || err == net.ErrClosed {
		return nil
	}
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	if isExpectedSocketClose(err) {
		return nil
	}
	if strings.Contains(err.Error(), "write on closed stream") {
		return nil
	}
	return err
}

func normalizeSetupError(operation string, err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) || isExpectedSocketClose(err) {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}
