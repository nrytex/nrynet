package relay

import (
	"errors"
	"io"
	"sync"
)

var errClientDisconnected = errors.New("client was disconnected")

type activeConnection struct {
	data    io.Closer
	visitor io.Closer
}

type clientConnections struct {
	mu          sync.Mutex
	generations map[string]uint64
	active      map[string]map[*activeConnection]struct{}
}

func newClientConnections() *clientConnections {
	return &clientConnections{
		generations: make(map[string]uint64),
		active:      make(map[string]map[*activeConnection]struct{}),
	}
}

func (c *clientConnections) generationFor(clientID string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generations[clientID]
}

func (c *clientConnections) register(
	clientID string,
	generation uint64,
	data io.Closer,
	visitor io.Closer,
) (func(), bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generations[clientID] != generation {
		return nil, false
	}
	connection := &activeConnection{data: data, visitor: visitor}
	if c.active[clientID] == nil {
		c.active[clientID] = make(map[*activeConnection]struct{})
	}
	c.active[clientID][connection] = struct{}{}
	return func() { c.remove(clientID, connection) }, true
}

func (c *clientConnections) remove(clientID string, connection *activeConnection) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.active[clientID], connection)
	if len(c.active[clientID]) == 0 {
		delete(c.active, clientID)
	}
}

func (c *clientConnections) disconnect(clientID string) {
	c.mu.Lock()
	c.generations[clientID]++
	connections := c.active[clientID]
	delete(c.active, clientID)
	c.mu.Unlock()
	for connection := range connections {
		_ = connection.data.Close()
		_ = connection.visitor.Close()
	}
}

func (b *Broker) relayPending(data DataStream, pending *pendingConn) error {
	unregister, ok := b.connections.register(
		pending.tunnel.ClientID,
		pending.connectionGeneration,
		data,
		pending.visitor,
	)
	if !ok {
		_ = data.Close()
		_ = pending.visitor.Close()
		return errClientDisconnected
	}
	defer unregister()
	return b.relay(data, pending.visitor, pending.onComplete)
}

func (b *Broker) DisconnectClient(clientID string) {
	b.connections.disconnect(clientID)
	var pending []*pendingConn
	b.mu.Lock()
	for requestID, entry := range b.pending {
		if entry.tunnel.ClientID != clientID {
			continue
		}
		delete(b.pending, requestID)
		pending = append(pending, entry)
	}
	b.mu.Unlock()
	for _, entry := range pending {
		_ = entry.visitor.Close()
		select {
		case entry.done <- errClientDisconnected:
		default:
		}
	}
}
