package api

import (
	"context"
	"net/http"
	"strconv"

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
type ClusterControl interface {
	Status(context.Context) (cluster.PublicState, bool, error)
	Create(context.Context, string, string) (cluster.PublicState, error)
	CreateJoinBundle(context.Context) (cluster.JoinBundle, error)
}

func registerCluster(mux *http.ServeMux, store ClusterStore, control ClusterControl) {
	if control != nil {
		mux.HandleFunc("GET /api/v1/cluster", func(w http.ResponseWriter, r *http.Request) {
			state, configured, err := control.Status(r.Context())
			if err != nil {
				failure(w, 503, "storage_unavailable", "Cluster state unavailable")
				return
			}
			respond(w, 200, map[string]any{"configured": configured, "cluster": state})
		})
		mux.HandleFunc("POST /api/v1/cluster", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Name           string `json:"name"`
				ControlAddress string `json:"control_address"`
			}
			if !readJSON(w, r, &in) {
				return
			}
			state, err := control.Create(r.Context(), in.Name, in.ControlAddress)
			if err != nil {
				failure(w, 409, "cluster_create_failed", err.Error())
				return
			}
			respond(w, 201, state)
		})
		mux.HandleFunc("POST /api/v1/cluster/join-tokens", func(w http.ResponseWriter, r *http.Request) {
			bundle, err := control.CreateJoinBundle(r.Context())
			if err != nil {
				failure(w, 409, "join_token_failed", err.Error())
				return
			}
			respond(w, 201, bundle)
		})
	}
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
		id := r.PathValue("id")
		if id == "" {
			failure(w, 400, "invalid_id", "Node ID required")
			return
		}
		if err := store.DeleteNode(r.Context(), id); err != nil {
			failure(w, 500, "storage_error", "Failed to delete node")
			return
		}
		respond(w, 204, nil)
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
