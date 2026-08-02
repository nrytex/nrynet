package agent

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"

	"github.com/nat-link/nat-link/internal/protocol"
)

func (a *Agent) handleOpenConnection(ctx context.Context, message protocol.ControlMessage) {
	if err := a.openAndRelay(ctx, message); err != nil {
		a.logger.Warn("connection relay failed", "request_id", message.RequestID, "error", err)
	}
}

func (a *Agent) openAndRelay(ctx context.Context, message protocol.ControlMessage) error {
	payload, err := protocol.DecodePayload[protocol.OpenConnectionPayload](message)
	if err != nil {
		return err
	}
	localAddress := net.JoinHostPort(payload.LocalHost, strconv.Itoa(payload.LocalPort))
	localConn, err := dialTCP(ctx, localAddress)
	if err != nil {
		return fmt.Errorf("dial local service: %w", err)
	}
	defer localConn.Close()
	dataConn, err := a.dialData(ctx)
	if err != nil {
		return fmt.Errorf("dial data channel: %w", err)
	}
	defer dataConn.Close()
	if err := writeHandshake(dataConn, a.options.Config.Token, a.options.Config.DeviceID, message.RequestID); err != nil {
		return err
	}
	return relay(dataConn, localConn, a.logger)
}

func (a *Agent) dialData(ctx context.Context) (net.Conn, error) {
	if !strings.HasPrefix(strings.ToLower(a.options.Config.ServerURL), "wss://") {
		return dialTCP(ctx, a.options.Config.DataAddress)
	}
	dialer := tls.Dialer{Config: &tls.Config{
		MinVersion: tls.VersionTLS13, InsecureSkipVerify: a.options.Config.InsecureSkipVerify,
	}}
	return dialer.DialContext(ctx, "tcp", a.options.Config.DataAddress)
}

func dialTCP(ctx context.Context, address string) (net.Conn, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func writeHandshake(writer io.Writer, token, deviceID, requestID string) error {
	buffered := bufio.NewWriter(writer)
	handshake := protocol.DataHandshake{Token: token, DeviceID: deviceID, RequestID: requestID}
	if err := json.NewEncoder(buffered).Encode(handshake); err != nil {
		return fmt.Errorf("write data handshake: %w", err)
	}
	return buffered.Flush()
}

func relay(left, right net.Conn, logger *slog.Logger) error {
	errCh := make(chan error, 2)
	go copyBytes(errCh, left, right)
	go copyBytes(errCh, right, left)
	firstErr := normalizeCopyError(<-errCh)
	_ = left.Close()
	_ = right.Close()
	secondErr := normalizeCopyError(<-errCh)
	if firstErr != nil {
		return firstErr
	}
	if secondErr != nil {
		logger.Debug("relay peer closed", "error", secondErr)
	}
	return nil
}

func copyBytes(errCh chan<- error, dst net.Conn, src net.Conn) {
	_, err := io.Copy(dst, src)
	errCh <- err
}

func normalizeCopyError(err error) error {
	if err == nil || err == io.EOF || err == net.ErrClosed {
		return nil
	}
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}
