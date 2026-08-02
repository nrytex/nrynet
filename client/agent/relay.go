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

	"github.com/nat-link/nat-link/internal/protocol"
)

func (a *Agent) handleOpenConnection(ctx context.Context, conn controlConn, message protocol.ControlMessage) {
	if err := a.openAndRelay(ctx, conn, message); err != nil {
		a.logger.Warn("connection relay failed", "request_id", message.RequestID, "error", err)
	}
}

func (a *Agent) openAndRelay(ctx context.Context, conn controlConn, message protocol.ControlMessage) error {
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
	dataConn, err := conn.openData(ctx, message.RequestID)
	if err != nil {
		return fmt.Errorf("dial data channel: %w", err)
	}
	defer dataConn.Close()
	if a.options.Config.Transport != "quic" {
		err = writeHandshake(dataConn, a.options.Config.Token, a.options.Config.DeviceID, message.RequestID)
	}
	if err != nil {
		return err
	}
	return a.relay(message.TunnelID, dataConn, localConn)
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

func (a *Agent) relay(tunnelID string, left dataConn, right net.Conn) error {
	errCh := make(chan error, 2)
	go copyBytes(errCh, left, right, func(n int64) {
		a.notifyTransfer(tunnelID, DirectionUpload, n)
	})
	go copyBytes(errCh, right, left, func(n int64) {
		a.notifyTransfer(tunnelID, DirectionDownload, n)
	})
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

func copyBytes(errCh chan<- error, dst io.Writer, src io.Reader, observe func(int64)) {
	written, err := io.Copy(dst, src)
	observe(written)
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
