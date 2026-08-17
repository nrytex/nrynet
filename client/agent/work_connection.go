package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/nrytex/nrynet/internal/protocol"
)

func supportsWorkConnections(conn controlConn) bool {
	supported, ok := conn.(interface{ supportsWorkConnections() bool })
	return ok && supported.supportsWorkConnections()
}

func (a *Agent) handleRequestedWorkConnection(ctx context.Context, control controlConn) {
	if err := a.runWorkConnection(a.relayContext(ctx), control); err != nil {
		a.logger.Debug("work connection ended", "error", err)
	}
}

func (a *Agent) runWorkConnection(ctx context.Context, control controlConn) error {
	openCtx, cancelOpen := context.WithTimeout(ctx, openConnectionSetupTimeout)
	data, err := control.openData(openCtx, "")
	if err != nil {
		cancelOpen()
		return fmt.Errorf("open pooled data connection: %w", err)
	}
	defer data.Close()
	stopClose := closeWorkConnectionOnContext(ctx, data)
	defer stopClose()
	handshake := protocol.DataHandshake{
		Token: a.options.Config.Token, DeviceID: a.options.Config.DeviceID,
		Role: protocol.DataRoleWorkConnection,
	}
	if err := writeDataHandshakeContext(openCtx, data, handshake); err != nil {
		cancelOpen()
		return err
	}
	cancelOpen()
	clearDataDeadline(data)
	buffered := newBufferedDataConn(data)
	assignment, err := readWorkAssignment(buffered)
	if err != nil {
		return err
	}
	setupCtx, cancelSetup := context.WithTimeout(ctx, openConnectionSetupTimeout)
	defer cancelSetup()
	setDataDeadline(setupCtx, buffered)
	local, err := a.dialAssignedLocalService(setupCtx, buffered, assignment)
	if err != nil {
		return err
	}
	defer local.Close()
	clearDataDeadline(buffered)
	return a.relay(ctx, assignment.TunnelID, buffered, local)
}

func (a *Agent) dialAssignedLocalService(
	ctx context.Context,
	data dataConn,
	assignment protocol.WorkConnectionAssignment,
) (net.Conn, error) {
	address := net.JoinHostPort(assignment.LocalHost, strconv.Itoa(assignment.LocalPort))
	local, err := a.dialLocalService(ctx, address)
	if err != nil {
		_ = writeWorkReady(data, protocol.WorkConnectionReady{Error: err.Error()})
		return nil, fmt.Errorf("dial assigned local service: %w", err)
	}
	if err := writeWorkReady(data, protocol.WorkConnectionReady{Ready: true}); err != nil {
		_ = local.Close()
		return nil, err
	}
	return local, nil
}

type bufferedDataConn struct {
	dataConn
	reader *bufio.Reader
}

func newBufferedDataConn(conn dataConn) *bufferedDataConn {
	return &bufferedDataConn{dataConn: conn, reader: bufio.NewReaderSize(conn, 4096)}
}

func (c *bufferedDataConn) Read(data []byte) (int, error) {
	return c.reader.Read(data)
}

func readWorkAssignment(conn *bufferedDataConn) (protocol.WorkConnectionAssignment, error) {
	line, err := conn.reader.ReadSlice('\n')
	if err != nil {
		return protocol.WorkConnectionAssignment{}, fmt.Errorf("read work assignment: %w", err)
	}
	var assignment protocol.WorkConnectionAssignment
	if err := json.Unmarshal(line, &assignment); err != nil {
		return assignment, fmt.Errorf("decode work assignment: %w", err)
	}
	if assignment.RequestID == "" || assignment.LocalHost == "" || assignment.LocalPort <= 0 {
		return assignment, fmt.Errorf("invalid work assignment")
	}
	return assignment, nil
}

func writeWorkReady(conn dataConn, ready protocol.WorkConnectionReady) error {
	if err := json.NewEncoder(conn).Encode(ready); err != nil {
		return fmt.Errorf("write work connection readiness: %w", err)
	}
	return nil
}

func setDataDeadline(ctx context.Context, conn dataConn) {
	setter, ok := conn.(interface{ SetDeadline(time.Time) error })
	if !ok {
		return
	}
	deadline := time.Now().Add(openConnectionSetupTimeout)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		deadline = value
	}
	_ = setter.SetDeadline(deadline)
}

func clearDataDeadline(conn dataConn) {
	if setter, ok := conn.(interface{ SetDeadline(time.Time) error }); ok {
		_ = setter.SetDeadline(time.Time{})
	}
}

func closeWorkConnectionOnContext(ctx context.Context, conn dataConn) func() {
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stop:
		}
	}()
	return func() { close(stop) }
}
