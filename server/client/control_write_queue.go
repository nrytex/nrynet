package client

import (
	"errors"
	"sync"

	"github.com/nrytex/nrynet/internal/protocol"
)

var (
	errControlWriteQueueClosed = errors.New("control write queue is closed")
	errControlWriteQueueFull   = errors.New("control write queue is full")
)

const (
	maxControlQueueMessages = 131072
	maxControlQueueBytes    = 512 << 20
)

type queuedControlMessage struct {
	message protocol.ControlMessage
	size    int
}

type controlWriteQueue struct {
	conn    ControlTransport
	onError func()

	mu           sync.Mutex
	condition    *sync.Cond
	urgent       []queuedControlMessage
	normal       []queuedControlMessage
	queuedCount  int
	queuedBytes  int
	urgentBudget int
	closed       bool
}

func newControlWriteQueue(conn ControlTransport, onError func()) *controlWriteQueue {
	queue := &controlWriteQueue{conn: conn, onError: onError}
	queue.condition = sync.NewCond(&queue.mu)
	go queue.run()
	return queue
}

func (q *controlWriteQueue) enqueue(message protocol.ControlMessage) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errControlWriteQueueClosed
	}
	item := queuedControlMessage{message: message, size: controlMessageSize(message)}
	urgent := isUrgentControlMessage(message)
	if q.queueFull(item.size) {
		return errControlWriteQueueFull
	}
	if q.closed {
		return errControlWriteQueueClosed
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

func (q *controlWriteQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.urgent = nil
	q.normal = nil
	q.queuedCount = 0
	q.queuedBytes = 0
	q.condition.Broadcast()
	q.mu.Unlock()
}

func (q *controlWriteQueue) run() {
	for {
		message, ok := q.next()
		if !ok {
			return
		}
		if err := q.conn.WriteJSON(message); err != nil {
			q.fail()
			return
		}
	}
}

func (q *controlWriteQueue) next() (protocol.ControlMessage, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.urgent) == 0 && len(q.normal) == 0 && !q.closed {
		q.condition.Wait()
	}
	if q.closed {
		return protocol.ControlMessage{}, false
	}
	var item queuedControlMessage
	if len(q.urgent) > 0 && (q.urgentBudget < 32 || len(q.normal) == 0) {
		item = q.urgent[0]
		q.urgent[0] = queuedControlMessage{}
		q.urgent = q.urgent[1:]
		q.urgentBudget++
	} else {
		item = q.normal[0]
		q.normal[0] = queuedControlMessage{}
		q.normal = q.normal[1:]
		q.urgentBudget = 0
	}
	q.queuedCount--
	q.queuedBytes -= item.size
	q.condition.Broadcast()
	return item.message, true
}

func (q *controlWriteQueue) fail() {
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
	q.condition.Broadcast()
	q.mu.Unlock()
	if q.onError != nil {
		q.onError()
	}
}

func (q *controlWriteQueue) queueFull(size int) bool {
	return q.queuedCount >= maxControlQueueMessages || q.queuedBytes+size > maxControlQueueBytes
}

func controlMessageSize(message protocol.ControlMessage) int {
	return len(message.Payload) + 128
}

func isUrgentControlMessage(message protocol.ControlMessage) bool {
	switch message.Type {
	case protocol.TypeHeartbeat, protocol.TypeHello, protocol.TypeOpenConnection,
		protocol.TypeP2PConnect, protocol.TypeTunnelSnapshot, protocol.TypeVisitorWebRTC,
		protocol.TypeConnectionFailed:
		return true
	default:
		return false
	}
}
