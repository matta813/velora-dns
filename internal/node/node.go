// Package node implements DNS node membership and health monitoring.
package node

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Node represents a DNS node in the cluster.
type Node struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Address      string    `json:"address"`
	Capabilities []string  `json:"capabilities"`
	Version      string    `json:"version"`
	Status       string    `json:"status"`
	LastSeenAt   time.Time `json:"last_seen_at"`
}

// NodeStore manages node persistence.
type NodeStore interface {
	SaveNode(ctx context.Context, node Node) error
	GetNode(ctx context.Context, id string) (Node, error)
	ListNodes(ctx context.Context) ([]Node, error)
	DeleteNode(ctx context.Context, id string) error
}

// Membership manages node registration and health monitoring.
type Membership struct {
	store     NodeStore
	localNode Node
	peers     map[string]*Peer
	mu        sync.RWMutex
	logger    Logger
}

// Logger is a simple logger interface.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Warn(msg string, args ...any)
}

// Peer represents a remote node connection.
type Peer struct {
	Node       Node
	LastPing   time.Time
	Healthy    bool
	CancelFunc context.CancelFunc
}

// NewMembership creates a new membership manager.
func NewMembership(store NodeStore, localNode Node, logger Logger) *Membership {
	return &Membership{
		store:     store,
		localNode: localNode,
		peers:     make(map[string]*Peer),
		logger:    logger,
	}
}

// Start begins membership management.
func (m *Membership) Start(ctx context.Context) {
	if err := m.store.SaveNode(ctx, m.localNode); err != nil {
		m.logger.Error("failed to register local node", "error", err)
		return
	}
	go m.healthCheckLoop(ctx)
}

// Stop stops all peer monitoring.
func (m *Membership) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, peer := range m.peers {
		if peer.CancelFunc != nil {
			peer.CancelFunc()
		}
	}
}

// AddPeer adds a remote node to monitor.
func (m *Membership) AddPeer(ctx context.Context, node Node) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.peers[node.ID]; exists {
		return
	}

	peerCtx, cancel := context.WithCancel(ctx)
	peer := &Peer{
		Node:       node,
		Healthy:    true,
		CancelFunc: cancel,
	}
	m.peers[node.ID] = peer

	go m.monitorPeer(peerCtx, peer)
}

// RemovePeer removes a remote node from monitoring.
func (m *Membership) RemovePeer(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if peer, exists := m.peers[id]; exists {
		if peer.CancelFunc != nil {
			peer.CancelFunc()
		}
		delete(m.peers, id)
	}
}

// GetPeers returns all monitored peers.
func (m *Membership) GetPeers() []Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	nodes := make([]Node, 0, len(m.peers))
	for _, peer := range m.peers {
		nodes = append(nodes, peer.Node)
	}
	return nodes
}

// IsHealthy returns true if the node is healthy.
func (m *Membership) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, peer := range m.peers {
		if !peer.Healthy {
			return false
		}
	}
	return true
}

func (m *Membership) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.updateLocalNode(ctx)
		}
	}
}

func (m *Membership) updateLocalNode(ctx context.Context) {
	m.localNode.LastSeenAt = time.Now()
	m.localNode.Status = "healthy"
	if err := m.store.SaveNode(ctx, m.localNode); err != nil {
		m.logger.Error("failed to update local node", "error", err)
	}
}

func (m *Membership) monitorPeer(ctx context.Context, peer *Peer) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pingPeer(ctx, peer)
		}
	}
}

func (m *Membership) pingPeer(ctx context.Context, peer *Peer) {
	peer.LastPing = time.Now()
	peer.Healthy = true

	if err := m.store.SaveNode(ctx, peer.Node); err != nil {
		m.logger.Error("failed to update peer", "node", peer.Node.ID, "error", err)
		peer.Healthy = false
	}
}

// GenerateNodeID generates a random node ID.
func GenerateNodeID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate node ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}
