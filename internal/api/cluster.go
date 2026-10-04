package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/matta813/velora-dns/internal/cluster"

	"github.com/matta813/velora-dns/internal/node"
	"github.com/matta813/velora-dns/internal/replication"
)

type ClusterStore interface {
	SaveNode(ctx context.Context, node node.Node) error
	GetNode(ctx context.Context, id string) (node.Node, error)
	ListNodes(ctx context.Context) ([]node.Node, error)
	DeleteNode(ctx context.Context, id string) error

	SaveConfigVersion(ctx context.Context, v replication.ConfigVersion) error
	GetLatestVersion(ctx context.Context) (replication.ConfigVersion, error)
	ListVersions(ctx context.Context, limit int) ([]replication.ConfigVersion, error)
}

func registerCluster(mux *http.ServeMux, store ClusterStore) {
	mux.HandleFunc("GET /api/v1/cluster/nodes", func(w http.ResponseWriter, r *http.Request) {
		nodes, err := store.ListNodes(r.Context())
		if err != nil {
			failure(w, 503, "storage_unavailable", "Cluster nodes unavailable")
			return
		}
		respond(w, 200, nodes)
	})

	mux.HandleFunc("GET /api/v1/cluster/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			failure(w, 400, "invalid_id", "Node ID required")
			return
		}
		node, err := store.GetNode(r.Context(), id)
		if err != nil {
			failure(w, 404, "not_found", "Node not found")
			return
		}
		respond(w, 200, node)
	})

	mux.HandleFunc("DELETE /api/v1/cluster/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		failure(w, http.StatusNotImplemented, "cluster_unavailable", "Cluster membership changes require a replication protocol")
	})

	mux.HandleFunc("GET /api/v1/cluster/config-versions", func(w http.ResponseWriter, r *http.Request) {
		limitStr := r.URL.Query().Get("limit")
		limit := 10
		if limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
			}
		}
		versions, err := store.ListVersions(r.Context(), limit)
		if err != nil {
			failure(w, 503, "storage_unavailable", "Config versions unavailable")
			return
		}
		respond(w, 200, versions)
	})

	mux.HandleFunc("GET /api/v1/cluster/config-versions/latest", func(w http.ResponseWriter, r *http.Request) {
		version, err := store.GetLatestVersion(r.Context())
		if err != nil {
			failure(w, 404, "not_found", "No config versions found")
			return
		}
		respond(w, 200, version)
	})
}

// ClusterManager is the primary/replica cluster service.
type ClusterManager interface {
	Overview(context.Context) (cluster.Overview, error)
	Create(ctx context.Context, name, advertisedURL string, allowInsecure, skipCheck bool) (cluster.State, error)
	IssueJoinToken(context.Context) (string, time.Time, error)
	Join(ctx context.Context, primaryURL, token, name string, allowInsecure bool) (cluster.State, error)
	SyncOnce(context.Context) error
	RemoveMember(context.Context, string) error
	Leave(context.Context) error
	Dissolve(context.Context) error
	ZonesReadOnly() bool
}

func clusterError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, cluster.ErrNotFound):
		failure(w, 404, "not_found", "Cluster node not found")
	case errors.Is(err, cluster.ErrState):
		failure(w, 409, "cluster_state", err.Error())
	case errors.Is(err, cluster.ErrConflict), errors.Is(err, cluster.ErrIncompatible):
		failure(w, 409, "cluster_conflict", err.Error())
	case errors.Is(err, cluster.ErrInvalid), errors.Is(err, cluster.ErrUnauthorized):
		failure(w, 400, "cluster_invalid", err.Error())
	default:
		failure(w, 503, "cluster_unavailable", "The cluster operation failed")
	}
}

func registerClusterManagement(mux *http.ServeMux, service ClusterManager) {
	overview := func(w http.ResponseWriter, r *http.Request, status int) {
		result, err := service.Overview(r.Context())
		if err != nil {
			clusterError(w, err)
			return
		}
		respond(w, status, result)
	}
	mux.HandleFunc("GET /api/v1/cluster/overview", func(w http.ResponseWriter, r *http.Request) { overview(w, r, 200) })
	mux.HandleFunc("POST /api/v1/cluster/create", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Name          string `json:"name"`
			AdvertisedURL string `json:"advertised_url"`
			AllowInsecure bool   `json:"allow_insecure"`
			SkipCheck     bool   `json:"skip_check"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		if _, err := service.Create(ctx, input.Name, input.AdvertisedURL, input.AllowInsecure, input.SkipCheck); err != nil {
			clusterError(w, err)
			return
		}
		overview(w, r, 201)
	})
	mux.HandleFunc("POST /api/v1/cluster/join-tokens", func(w http.ResponseWriter, r *http.Request) {
		token, expires, err := service.IssueJoinToken(r.Context())
		if err != nil {
			clusterError(w, err)
			return
		}
		respond(w, 201, map[string]any{"token": token, "expires_at": expires})
	})
	mux.HandleFunc("POST /api/v1/cluster/connect", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			PrimaryURL    string `json:"primary_url"`
			Token         string `json:"token"`
			Name          string `json:"name"`
			AllowInsecure bool   `json:"allow_insecure"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if _, err := service.Join(ctx, input.PrimaryURL, input.Token, input.Name, input.AllowInsecure); err != nil {
			clusterError(w, err)
			return
		}
		overview(w, r, 200)
	})
	mux.HandleFunc("POST /api/v1/cluster/sync", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		// A failed sync is reported in the overview, not as an HTTP error.
		if err := service.SyncOnce(ctx); errors.Is(err, cluster.ErrState) {
			clusterError(w, err)
			return
		}
		overview(w, r, 200)
	})
	mux.HandleFunc("DELETE /api/v1/cluster/members/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := service.RemoveMember(r.Context(), r.PathValue("id")); err != nil {
			clusterError(w, err)
			return
		}
		overview(w, r, 200)
	})
	mux.HandleFunc("POST /api/v1/cluster/leave", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		if err := service.Leave(ctx); err != nil {
			clusterError(w, err)
			return
		}
		overview(w, r, 200)
	})
	mux.HandleFunc("POST /api/v1/cluster/dissolve", func(w http.ResponseWriter, r *http.Request) {
		if err := service.Dissolve(r.Context()); err != nil {
			clusterError(w, err)
			return
		}
		overview(w, r, 200)
	})
}
