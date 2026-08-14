package agent

import (
	"context"
	"errors"
	"sync"

	"github.com/nrytex/nrynet/internal/protocol"
)

var (
	errAgentControlWriteQueueClosed = errors.New("agent control write queue is closed")
	errAgentControlQueueFull        = errors.New("agent control write queue is full")
)

const (
	maxAgentControlQueueMessages = 65536
	maxAgentControlQueueBytes    = 256 << 20
)

type queuedAgentMessage struct {
	value any
	size  int
}

// queuedControlConn keeps relay workers away from the control socket's write
// syscall. This matters most for UDP and visitor traffic: a slow server write
// must not block the heartbeat or every other active tunnel.
type queuedControlConn struct {
	base controlConn

	mu              sync.Mutex
	condition       *sync.Cond
	urgent          []queuedAgentMessage
	normal          []queuedAgentMessage
	queuedCount     int
	queuedBytes     int
	urgentBudget    int
	heartbeatQueued bool
	closed          bool
}

func newQueuedControlConn(base controlConn) *queuedControlConn {
	queue := &queuedControlConn{base: base}
	queue.condition = sync.NewCond(&queue.mu)
	go queue.run()
	return queue
}

func (q *queuedControlConn) readJSON(value any) error {
	return q.base.readJSON(value)
}

func (q *queuedControlConn) writeJSON(value any) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errAgentControlWriteQueueClosed
	}
	item := queuedAgentMessage{value: value, size: agentControlMessageSize(value)}
	urgent := isUrgentControlMessage(value)
	if q.queueFull(item.size) {
		return errAgentControlQueueFull
	}
	if valueIsHeartbeat(value) {
		if q.heartbeatQueued {
			return nil
		}
		q.heartbeatQueued = true
	}
	if q.closed {
		if valueIsHeartbeat(value) {
			q.heartbeatQueued = false
		}
		return errAgentControlWriteQueueClosed
	}
	if urgent {
		q.urgent = append(q.urgent, item)
	} else {
		q.normal = append(q.normal, item)
	}
	q.queuedCount++
	q.queuedBytes += item.size
	q.condition.Signal()
	return nil
}

func (q *queuedControlConn) close() error {
	q.mu.Lock()
	q.closed = true
	q.urgent = nil
	q.normal = nil
	q.queuedCount = 0
	q.queuedBytes = 0
	q.heartbeatQueued = false
	q.condition.Broadcast()
	q.mu.Unlock()
	return q.base.close()
}

func (q *queuedControlConn) isClosed() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.closed
}

func (q *queuedControlConn) openData(ctx context.Context, requestID string) (dataConn, error) {
	return q.base.openData(ctx, requestID)
}

func (q *queuedControlConn) singleDataOpen() bool {
	single, ok := q.base.(interface{ singleDataOpen() bool })
	return ok && single.singleDataOpen()
}

func (q *queuedControlConn) ping() error {
	pinger, ok := q.base.(interface{ ping() error })
	if !ok {
		return nil
	}
	return pinger.ping()
}

func (q *queuedControlConn) run() {
	for {
		value, ok := q.next()
		if !ok {
			return
		}
		if err := q.base.writeJSON(value); err != nil {
			q.fail()
			return
		}
	}
}

func (q *queuedControlConn) next() (any, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.urgent) == 0 && len(q.normal) == 0 && !q.closed {
		q.condition.Wait()
	}
	if q.closed {
		return nil, false
	}
	if len(q.urgent) > 0 && (q.urgentBudget < 32 || len(q.normal) == 0) {
		item := q.urgent[0]
		q.urgent[0] = queuedAgentMessage{}
		q.urgent = q.urgent[1:]
		q.queuedCount--
		q.queuedBytes -= item.size
		if valueIsHeartbeat(item.value) {
			q.heartbeatQueued = false
		}
		q.condition.Broadcast()
		q.urgentBudget++
		return item.value, true
	}
	item := q.normal[0]
	q.normal[0] = queuedAgentMessage{}
	q.normal = q.normal[1:]
	q.queuedCount--
	q.queuedBytes -= item.size
	q.condition.Broadcast()
	q.urgentBudget = 0
	return item.value, true
}

func (q *queuedControlConn) fail() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	q.urgent = nil
	q.normal = nil
	q.queuedCount = 0
	q.queuedBytes = 0
	q.heartbeatQueued = false
	q.condition.Broadcast()
	q.mu.Unlock()
	_ = q.base.close()
}

func isUrgentControlMessage(value any) bool {
	message, ok := value.(protocol.ControlMessage)
	return ok && (message.Type == protocol.TypeHeartbeat || message.Type == protocol.TypeHello ||
		message.Type == protocol.TypeOpenConnection || message.Type == protocol.TypeP2PConnect ||
		message.Type == protocol.TypeTunnelSnapshot ||
		message.Type == protocol.TypeVisitorWebRTC || message.Type == protocol.TypeConnectionFailed)
}

func agentControlMessageSize(value any) int {
	message, ok := value.(protocol.ControlMessage)
	if !ok {
		return 256
	}
	return len(message.Payload) + 128
}

func (q *queuedControlConn) queueFull(size int) bool {
	return q.queuedCount >= maxAgentControlQueueMessages || q.queuedBytes+size > maxAgentControlQueueBytes
}

func valueIsHeartbeat(value any) bool {
	message, ok := value.(protocol.ControlMessage)
	return ok && message.Type == protocol.TypeHeartbeat
}
