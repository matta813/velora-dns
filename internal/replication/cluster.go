// Package replication implements central multi-node management.
package replication

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ClusterManager manages multiple DNS nodes from a central point.
type ClusterManager struct {
	membership MembershipProvider
	config     ConfigReplicatorProvider
	logger     Logger
	nodes      map[string]*ManagedNode
	mu         sync.RWMutex
}

// ManagedNode represents a node managed by the central manager.
type ManagedNode struct {
	ID             string
	Name           string
	Address        string
	Status         string
	LastHeartbeat  time.Time
	ConfigVersion  int
}

// MembershipProvider is the interface for node membership.
type MembershipProvider interface {
	GetPeers() []ManagedNodeInfo
	IsHealthy() bool
}

// ManagedNodeInfo is the info returned by membership.
type ManagedNodeInfo struct {
	ID      string
	Name    string
	Address string
	Status  string
}

// ConfigReplicatorProvider is the interface for config replication.
type ConfigReplicatorProvider interface {
	Replicate(ctx context.Context, config any, appliedBy string) (*ConfigVersion, error)
	GetCurrentVersion() int
}

// NewClusterManager creates a new central cluster manager.
func NewClusterManager(membership MembershipProvider, config ConfigReplicatorProvider, logger Logger) *ClusterManager {
	return &ClusterManager{
		membership: membership,
		config:     config,
		logger:     logger,
		nodes:      make(map[string]*ManagedNode),
	}
}

// Start begins cluster management.
func (m *ClusterManager) Start(ctx context.Context) {
	go m.heartbeatLoop(ctx)
}

func (m *ClusterManager) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.updateNodes(ctx)
		}
	}
}

func (m *ClusterManager) updateNodes(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	peers := m.membership.GetPeers()
	for _, peer := range peers {
		node, exists := m.nodes[peer.ID]
		if !exists {
			node = &ManagedNode{
				ID:    peer.ID,
				Name:  peer.Name,
				Address: peer.Address,
			}
			m.nodes[peer.ID] = node
		}
		node.Status = peer.Status
		node.LastHeartbeat = time.Now()
	}
}

// GetNodes returns all managed nodes.
func (m *ClusterManager) GetNodes() []*ManagedNode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*ManagedNode, 0, len(m.nodes))
	for _, node := range m.nodes {
		out = append(out, node)
	}
	return out
}

// GetNode returns a specific managed node.
func (m *ClusterManager) GetNode(id string) (*ManagedNode, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	node, ok := m.nodes[id]
	return node, ok
}

// RolloutConfig pushes a configuration update to all nodes.
func (m *ClusterManager) RolloutConfig(ctx context.Context, config any) error {
	nodes := m.GetNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes available")
	}

	for _, node := range nodes {
		m.logger.Info("rolling out config", "node", node.ID, "name", node.Name)
	}

	_, err := m.config.Replicate(ctx, config, "central-manager")
	if err != nil {
		return fmt.Errorf("replicate config: %w", err)
	}

	return nil
}
