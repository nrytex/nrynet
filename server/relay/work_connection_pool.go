package relay

import (
	"net"
	"sync"
)

const defaultWorkConnectionPoolSize = 8

type workConnectionTransport interface {
	net.Conn
	readJSONLine(any) error
}

type idleWorkConnection struct {
	conn       workConnectionTransport
	clientID   string
	generation uint64
}

type workConnectionPool struct {
	mu      sync.Mutex
	maxIdle int
	idle    map[string][]idleWorkConnection
}

func newWorkConnectionPool(maxIdle int) *workConnectionPool {
	return &workConnectionPool{maxIdle: maxIdle, idle: make(map[string][]idleWorkConnection)}
}

func (p *workConnectionPool) add(worker idleWorkConnection) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	workers := p.idle[worker.clientID]
	if len(workers) >= p.maxIdle {
		return false
	}
	p.idle[worker.clientID] = append(workers, worker)
	return true
}

func (p *workConnectionPool) take(clientID string) (idleWorkConnection, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	workers := p.idle[clientID]
	if len(workers) == 0 {
		return idleWorkConnection{}, false
	}
	last := len(workers) - 1
	worker := workers[last]
	workers[last] = idleWorkConnection{}
	workers = workers[:last]
	if len(workers) == 0 {
		delete(p.idle, clientID)
	} else {
		p.idle[clientID] = workers
	}
	return worker, true
}

func (p *workConnectionPool) closeClient(clientID string) {
	p.mu.Lock()
	workers := p.idle[clientID]
	delete(p.idle, clientID)
	p.mu.Unlock()
	for _, worker := range workers {
		_ = worker.conn.Close()
	}
}

func (p *workConnectionPool) closeAll() {
	p.mu.Lock()
	all := p.idle
	p.idle = make(map[string][]idleWorkConnection)
	p.mu.Unlock()
	for _, workers := range all {
		for _, worker := range workers {
			_ = worker.conn.Close()
		}
	}
}

func (p *workConnectionPool) target() int {
	return p.maxIdle
}

func (p *workConnectionPool) count(clientID string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.idle[clientID])
}
