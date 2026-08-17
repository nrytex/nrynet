package client

import (
	"context"
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
	done    chan error
}

type controlWriteQueue struct {
	conn    ControlTransport
	onError func()

	mu           sync.Mutex
	condition    *sync.Cond
	critical     []queuedControlMessage
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
	return q.enqueueItem(queuedControlMessage{message: message, size: controlMessageSize(message)})
}

func (q *controlWriteQueue) enqueueWait(ctx context.Context, message protocol.ControlMessage) error {
	done := make(chan error, 1)
	if err := q.enqueueItem(queuedControlMessage{
		message: message,
		size:    controlMessageSize(message),
		done:    done,
	}); err != nil {
		return err
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *controlWriteQueue) enqueueItem(item queuedControlMessage) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errControlWriteQueueClosed
	}
	critical := isCriticalControlMessage(item.message)
	urgent := isUrgentControlMessage(item.message)
	if !critical && q.queueFull(item.size) {
		return errControlWriteQueueFull
	}
	if q.closed {
		return errControlWriteQueueClosed
	}
	if critical {
		q.critical = append(q.critical, item)
	} else if urgent {
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
	pending := append(q.critical, q.urgent...)
	pending = append(pending, q.normal...)
	q.closed = true
	q.critical = nil
	q.urgent = nil
	q.normal = nil
	q.queuedCount = 0
	q.queuedBytes = 0
	q.condition.Broadcast()
	q.mu.Unlock()
	completeQueuedMessages(pending, errControlWriteQueueClosed)
}

func (q *controlWriteQueue) run() {
	for {
		message, ok := q.next()
		if !ok {
			return
		}
		err := q.conn.WriteJSON(message.message)
		completeQueuedMessage(message, err)
		if err != nil {
			q.fail(err)
			return
		}
	}
}

func (q *controlWriteQueue) next() (queuedControlMessage, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.critical) == 0 && len(q.urgent) == 0 && len(q.normal) == 0 && !q.closed {
		q.condition.Wait()
	}
	if q.closed {
		return queuedControlMessage{}, false
	}
	var item queuedControlMessage
	if len(q.critical) > 0 {
		item = q.critical[0]
		q.critical[0] = queuedControlMessage{}
		q.critical = q.critical[1:]
	} else if len(q.urgent) > 0 && (q.urgentBudget < 32 || len(q.normal) == 0) {
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
	return item, true
}

func (q *controlWriteQueue) fail(cause error) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	pending := append(q.critical, q.urgent...)
	pending = append(pending, q.normal...)
	q.closed = true
	q.critical = nil
	q.urgent = nil
	q.normal = nil
	q.queuedCount = 0
	q.queuedBytes = 0
	q.condition.Broadcast()
	q.mu.Unlock()
	completeQueuedMessages(pending, cause)
	if q.onError != nil {
		q.onError()
	}
}

func completeQueuedMessage(message queuedControlMessage, err error) {
	if message.done != nil {
		message.done <- err
	}
}

func completeQueuedMessages(messages []queuedControlMessage, err error) {
	for _, message := range messages {
		completeQueuedMessage(message, err)
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
		protocol.TypeRequestWorkConn,
		protocol.TypeP2PConnect, protocol.TypeTunnelSnapshot, protocol.TypeVisitorWebRTC,
		protocol.TypeConnectionFailed, protocol.TypeHeartbeatAck:
		return true
	default:
		return false
	}
}

func isCriticalControlMessage(message protocol.ControlMessage) bool {
	return message.Type == protocol.TypeHeartbeat || message.Type == protocol.TypeHeartbeatAck
}
