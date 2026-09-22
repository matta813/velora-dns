package cluster

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"time"
)

const joinTokenLifetime = 15 * time.Minute

type Store interface {
	SaveClusterState(context.Context, State) error
	GetClusterState(context.Context) (State, error)
	HasClusterState(context.Context) (bool, error)
	CreateClusterJoinToken(context.Context, []byte, time.Time) error
	ConsumeClusterJoinToken(context.Context, []byte) error
}

type Controller struct {
	store Store
	now   func() time.Time
}

func NewController(store Store) *Controller { return &Controller{store: store, now: time.Now} }

type PublicState struct {
	ClusterID      string `json:"cluster_id"`
	NodeID         string `json:"node_id"`
	NodeName       string `json:"node_name"`
	ControlAddress string `json:"control_address"`
	Role           string `json:"role"`
}

func publicState(s State) PublicState {
	return PublicState{ClusterID: s.ClusterID, NodeID: s.NodeID, NodeName: s.NodeName, ControlAddress: s.ControlAddress, Role: s.Role}
}

func (c *Controller) Status(ctx context.Context) (PublicState, bool, error) {
	s, err := c.store.GetClusterState(ctx)
	if err != nil {
		ok, checkErr := c.store.HasClusterState(ctx)
		if checkErr != nil {
			return PublicState{}, false, checkErr
		}
		return PublicState{}, ok, nil
	}
	return publicState(s), true, nil
}

func (c *Controller) Create(ctx context.Context, nodeName, controlAddress string) (PublicState, error) {
	if nodeName == "" {
		return PublicState{}, fmt.Errorf("node name is required")
	}
	if _, _, err := net.SplitHostPort(controlAddress); err != nil {
		return PublicState{}, fmt.Errorf("invalid control address: %w", err)
	}
	exists, err := c.store.HasClusterState(ctx)
	if err != nil {
		return PublicState{}, err
	}
	if exists {
		return PublicState{}, fmt.Errorf("cluster already exists")
	}
	clusterID, err := randomID()
	if err != nil {
		return PublicState{}, err
	}
	nodeID, err := randomID()
	if err != nil {
		return PublicState{}, err
	}
	now := c.now().UTC()
	authority, err := NewAuthority(clusterID, now)
	if err != nil {
		return PublicState{}, err
	}
	certificate, key, err := IssueNodeIdentity(authority, clusterID, nodeID, controlAddress, now)
	if err != nil {
		return PublicState{}, err
	}
	state := State{ClusterID: clusterID, NodeID: nodeID, NodeName: nodeName, ControlAddress: controlAddress, Role: "leader", CACertificate: authority.CertificatePEM, Certificate: certificate, PrivateKey: key, CreatedAt: now}
	if err := c.store.SaveClusterState(ctx, state); err != nil {
		return PublicState{}, err
	}
	return publicState(state), nil
}

func (c *Controller) CreateJoinBundle(ctx context.Context) (JoinBundle, error) {
	state, err := c.store.GetClusterState(ctx)
	if err != nil {
		return JoinBundle{}, fmt.Errorf("load cluster state: %w", err)
	}
	if state.Role != "leader" {
		return JoinBundle{}, fmt.Errorf("only the cluster leader can issue join tokens")
	}
	raw, digest, err := NewJoinToken()
	if err != nil {
		return JoinBundle{}, err
	}
	expires := c.now().Add(joinTokenLifetime).UTC()
	if err = c.store.CreateClusterJoinToken(ctx, digest, expires); err != nil {
		return JoinBundle{}, err
	}
	return JoinBundle{LeaderAddress: state.ControlAddress, CACertificate: state.CACertificate, Token: raw, ExpiresAt: expires}, nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
