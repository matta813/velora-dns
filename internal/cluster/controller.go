package cluster

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/matta813/velora-dns/internal/node"
)

const joinTokenLifetime = 15 * time.Minute

type Store interface {
	SaveClusterState(context.Context, State) error
	GetClusterState(context.Context) (State, error)
	HasClusterState(context.Context) (bool, error)
	CreateClusterJoinToken(context.Context, []byte, time.Time) error
	ConsumeClusterJoinToken(context.Context, []byte) error
	SaveNode(context.Context, node.Node) error
}

type JoinRequest struct {
	Token          string `json:"token"`
	NodeID         string `json:"node_id"`
	NodeName       string `json:"node_name"`
	ControlAddress string `json:"control_address"`
	CSR            []byte `json:"csr"`
}
type JoinResponse struct {
	ClusterID     string `json:"cluster_id"`
	LeaderAddress string `json:"leader_address"`
	CACertificate []byte `json:"ca_certificate"`
	Certificate   []byte `json:"certificate"`
}

func (c *Controller) AcceptJoin(ctx context.Context, request JoinRequest) (JoinResponse, error) {
	state, err := c.store.GetClusterState(ctx)
	if err != nil {
		return JoinResponse{}, fmt.Errorf("load cluster state: %w", err)
	}
	if state.Role != "leader" {
		return JoinResponse{}, fmt.Errorf("only the cluster leader accepts joins")
	}
	if request.Token == "" || request.NodeID == "" || request.NodeName == "" || request.ControlAddress == "" || len(request.CSR) == 0 {
		return JoinResponse{}, fmt.Errorf("invalid join request")
	}
	if err = c.store.ConsumeClusterJoinToken(ctx, TokenDigest(request.Token)); err != nil {
		return JoinResponse{}, err
	}
	certificate, err := SignCSR(Authority{CertificatePEM: state.CACertificate, PrivateKeyPEM: state.PrivateKey}, state.ClusterID, request.NodeID, request.ControlAddress, request.CSR, c.now())
	if err != nil {
		return JoinResponse{}, err
	}
	if err = c.store.SaveNode(ctx, node.Node{ID: request.NodeID, Name: request.NodeName, Address: request.ControlAddress, Status: "healthy", LastSeenAt: c.now().UTC()}); err != nil {
		return JoinResponse{}, fmt.Errorf("save joined node: %w", err)
	}
	return JoinResponse{ClusterID: state.ClusterID, LeaderAddress: state.ControlAddress, CACertificate: state.CACertificate, Certificate: certificate}, nil
}

type Controller struct {
	store      Store
	now        func() time.Time
	mu         sync.Mutex
	runtimeCtx context.Context
	server     *http.Server
	listener   net.Listener
}

func NewController(store Store) *Controller { return &Controller{store: store, now: time.Now} }

func (c *Controller) Start(ctx context.Context) error {
	c.mu.Lock()
	c.runtimeCtx = ctx
	c.mu.Unlock()
	state, err := c.store.GetClusterState(ctx)
	if err != nil {
		configured, checkErr := c.store.HasClusterState(ctx)
		if checkErr != nil {
			return checkErr
		}
		if !configured {
			return nil
		}
		return err
	}
	return c.startBootstrap(state)
}

func (c *Controller) Stop(ctx context.Context) error {
	c.mu.Lock()
	server := c.server
	c.server = nil
	c.listener = nil
	c.mu.Unlock()
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

func (c *Controller) startBootstrap(state State) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.server != nil {
		return nil
	}
	config, err := BootstrapTLSConfig(state)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", state.ControlAddress)
	if err != nil {
		return fmt.Errorf("listen on cluster control address: %w", err)
	}
	tlsListener := tls.NewListener(listener, config)
	server := &http.Server{Handler: BootstrapHandler(c), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	c.server = server
	c.listener = tlsListener
	go func() { _ = server.Serve(tlsListener) }()
	return nil
}

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
	c.mu.Lock()
	runtimeReady := c.runtimeCtx != nil
	c.mu.Unlock()
	if runtimeReady {
		if err := c.startBootstrap(state); err != nil {
			return PublicState{}, err
		}
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
