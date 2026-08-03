package advanced

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/nrytex/nrynet/internal/config"
)

var ErrNoHealthyRelay = errors.New("no healthy relay node available")

type RelayNode struct {
	ID          string    `json:"id"`
	Address     string    `json:"address"`
	ControlAddr string    `json:"control_address,omitempty"`
	Connections int       `json:"connections"`
	Healthy     bool      `json:"healthy"`
	LastSeen    time.Time `json:"last_seen"`
}

type TunnelAssignment struct {
	TunnelID string `json:"tunnel_id"`
	NodeID   string `json:"node_id"`
	Address  string `json:"address"`
}

type RelayRegistry struct {
	mu          sync.Mutex
	timeout     time.Duration
	nodes       map[string]RelayNode
	assignments map[string]string
}

func NewRelayRegistry(timeout time.Duration) *RelayRegistry {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &RelayRegistry{
		timeout:     timeout,
		nodes:       make(map[string]RelayNode),
		assignments: make(map[string]string),
	}
}

func (r *RelayRegistry) Register(node RelayNode) (RelayNode, error) {
	if node.ID == "" || node.Address == "" {
		return RelayNode{}, errors.New("relay id and address are required")
	}
	if node.ControlAddr != "" {
		if err := config.ValidateSecureHTTPURL(node.ControlAddr); err != nil {
			return RelayNode{}, err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	node.Healthy = true
	node.LastSeen = time.Now()
	r.nodes[node.ID] = node
	return node, nil
}

func (r *RelayRegistry) Heartbeat(id string, connections int) (RelayNode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	node, ok := r.nodes[id]
	if !ok {
		return RelayNode{}, errors.New("relay node not found")
	}
	node.Connections = connections
	node.Healthy = true
	node.LastSeen = time.Now()
	r.nodes[id] = node
	return node, nil
}

func (r *RelayRegistry) AssignTunnel(tunnelID string) (TunnelAssignment, error) {
	return r.assign(tunnelID, "")
}

func (r *RelayRegistry) ReassignTunnel(tunnelID string) (TunnelAssignment, error) {
	r.mu.Lock()
	exclude := r.assignments[tunnelID]
	r.mu.Unlock()
	return r.assign(tunnelID, exclude)
}

func (r *RelayRegistry) Assignment(tunnelID string) (TunnelAssignment, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(time.Now())
	id, ok := r.assignments[tunnelID]
	if !ok {
		return TunnelAssignment{}, false
	}
	node, ok := r.nodes[id]
	return TunnelAssignment{TunnelID: tunnelID, NodeID: id, Address: node.Address}, ok
}

func (r *RelayRegistry) Assignments() []TunnelAssignment {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(time.Now())
	items := make([]TunnelAssignment, 0, len(r.assignments))
	for tunnelID, nodeID := range r.assignments {
		node := r.nodes[nodeID]
		items = append(items, TunnelAssignment{TunnelID: tunnelID, NodeID: nodeID, Address: node.Address})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].TunnelID < items[j].TunnelID })
	return items
}

func (r *RelayRegistry) IsAssignedTo(tunnelID, nodeID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(time.Now())
	node, ok := r.nodes[nodeID]
	return ok && node.Healthy && r.assignments[tunnelID] == nodeID
}

func (r *RelayRegistry) UnhealthyAssignments() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(time.Now())
	var tunnels []string
	for tunnelID, nodeID := range r.assignments {
		if node, ok := r.nodes[nodeID]; !ok || !node.Healthy {
			tunnels = append(tunnels, tunnelID)
		}
	}
	sort.Strings(tunnels)
	return tunnels
}

func (r *RelayRegistry) ReleaseTunnel(tunnelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.assignments[tunnelID]
	r.decrement(id)
	delete(r.assignments, tunnelID)
}

func (r *RelayRegistry) MarkUnhealthy(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	node, ok := r.nodes[id]
	if !ok {
		return
	}
	node.Healthy = false
	r.nodes[id] = node
}

func (r *RelayRegistry) Nodes() []RelayNode {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(time.Now())
	nodes := make([]RelayNode, 0, len(r.nodes))
	for _, node := range r.nodes {
		nodes = append(nodes, node)
	}
	sortNodes(nodes)
	return nodes
}

func (r *RelayRegistry) Node(id string) (RelayNode, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(time.Now())
	node, ok := r.nodes[id]
	return node, ok
}

func (r *RelayRegistry) assign(tunnelID, exclude string) (TunnelAssignment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(time.Now())
	node, ok := r.pickLocked(exclude)
	if !ok {
		return TunnelAssignment{}, ErrNoHealthyRelay
	}
	r.decrement(r.assignments[tunnelID])
	node.Connections++
	r.nodes[node.ID] = node
	r.assignments[tunnelID] = node.ID
	return TunnelAssignment{TunnelID: tunnelID, NodeID: node.ID, Address: node.Address}, nil
}

func (r *RelayRegistry) pickLocked(exclude string) (RelayNode, bool) {
	nodes := make([]RelayNode, 0, len(r.nodes))
	for _, node := range r.nodes {
		if node.Healthy && node.ID != exclude {
			nodes = append(nodes, node)
		}
	}
	sortNodes(nodes)
	if len(nodes) == 0 {
		return RelayNode{}, false
	}
	return nodes[0], true
}

func (r *RelayRegistry) expireLocked(now time.Time) {
	for id, node := range r.nodes {
		if now.Sub(node.LastSeen) <= r.timeout {
			continue
		}
		node.Healthy = false
		r.nodes[id] = node
	}
}

func (r *RelayRegistry) decrement(id string) {
	node, ok := r.nodes[id]
	if !ok || node.Connections == 0 {
		return
	}
	node.Connections--
	r.nodes[id] = node
}

func sortNodes(nodes []RelayNode) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Healthy != nodes[j].Healthy {
			return nodes[i].Healthy
		}
		if nodes[i].Connections != nodes[j].Connections {
			return nodes[i].Connections < nodes[j].Connections
		}
		return nodes[i].ID < nodes[j].ID
	})
}
