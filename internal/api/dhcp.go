package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/matta813/velora-dns/internal/dhcp"
)

type DHCPStore interface {
	CreatePool(ctx context.Context, pool *dhcp.Pool) error
	GetPool(ctx context.Context, id int64) (*dhcp.Pool, error)
	ListPools(ctx context.Context) ([]dhcp.Pool, error)
	UpdatePool(ctx context.Context, pool *dhcp.Pool) error
	DeletePool(ctx context.Context, id int64) error

	CreateReservation(ctx context.Context, r *dhcp.Reservation) error
	ListReservations(ctx context.Context, poolID int64) ([]dhcp.Reservation, error)
	DeleteReservation(ctx context.Context, id int64) error

	SaveLease(ctx context.Context, l *dhcp.Lease) error
	GetLease(ctx context.Context, poolID int64, mac string) (*dhcp.Lease, error)
	ListLeases(ctx context.Context, poolID int64) ([]dhcp.Lease, error)
	ListActiveLeases(ctx context.Context) ([]dhcp.Lease, error)
	DeleteLease(ctx context.Context, id int64) error
}

func registerDHCP(mux *http.ServeMux, store DHCPStore) {
	mux.HandleFunc("GET /api/v1/dhcp/pools", func(w http.ResponseWriter, r *http.Request) {
		pools, err := store.ListPools(r.Context())
		if err != nil {
			failure(w, 503, "storage_unavailable", "DHCP pools unavailable")
			return
		}
		respond(w, 200, pools)
	})

	mux.HandleFunc("POST /api/v1/dhcp/pools", func(w http.ResponseWriter, r *http.Request) {
		var pool dhcp.Pool
		if !readJSON(w, r, &pool) {
			return
		}
		if err := dhcp.ValidatePool(&pool); err != nil {
			failure(w, 400, "invalid_pool", err.Error())
			return
		}
		pool.Enabled = true
		if err := store.CreatePool(r.Context(), &pool); err != nil {
			failure(w, 500, "storage_error", "Failed to create pool")
			return
		}
		respond(w, 201, pool)
	})

	mux.HandleFunc("GET /api/v1/dhcp/pools/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			failure(w, 400, "invalid_id", "Invalid pool ID")
			return
		}
		pool, err := store.GetPool(r.Context(), id)
		if err != nil {
			failure(w, 404, "not_found", "Pool not found")
			return
		}
		respond(w, 200, pool)
	})

	mux.HandleFunc("PUT /api/v1/dhcp/pools/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			failure(w, 400, "invalid_id", "Invalid pool ID")
			return
		}
		var pool dhcp.Pool
		if !readJSON(w, r, &pool) {
			return
		}
		pool.ID = id
		if err := dhcp.ValidatePool(&pool); err != nil {
			failure(w, 400, "invalid_pool", err.Error())
			return
		}
		if err := store.UpdatePool(r.Context(), &pool); err != nil {
			failure(w, 500, "storage_error", "Failed to update pool")
			return
		}
		respond(w, 200, pool)
	})

	mux.HandleFunc("DELETE /api/v1/dhcp/pools/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			failure(w, 400, "invalid_id", "Invalid pool ID")
			return
		}
		if err := store.DeletePool(r.Context(), id); err != nil {
			failure(w, 500, "storage_error", "Failed to delete pool")
			return
		}
		respond(w, 204, nil)
	})

	mux.HandleFunc("GET /api/v1/dhcp/pools/{id}/reservations", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			failure(w, 400, "invalid_id", "Invalid pool ID")
			return
		}
		reservations, err := store.ListReservations(r.Context(), id)
		if err != nil {
			failure(w, 503, "storage_unavailable", "Reservations unavailable")
			return
		}
		respond(w, 200, reservations)
	})

	mux.HandleFunc("POST /api/v1/dhcp/pools/{id}/reservations", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			failure(w, 400, "invalid_id", "Invalid pool ID")
			return
		}
		var reservation dhcp.Reservation
		if !readJSON(w, r, &reservation) {
			return
		}
		reservation.PoolID = id
		if err := dhcp.ValidateMAC(reservation.MACAddress); err != nil {
			failure(w, 400, "invalid_mac", err.Error())
			return
		}
		if err := store.CreateReservation(r.Context(), &reservation); err != nil {
			failure(w, 500, "storage_error", "Failed to create reservation")
			return
		}
		respond(w, 201, reservation)
	})

	mux.HandleFunc("DELETE /api/v1/dhcp/reservations/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			failure(w, 400, "invalid_id", "Invalid reservation ID")
			return
		}
		if err := store.DeleteReservation(r.Context(), id); err != nil {
			failure(w, 500, "storage_error", "Failed to delete reservation")
			return
		}
		respond(w, 204, nil)
	})

	mux.HandleFunc("GET /api/v1/dhcp/leases", func(w http.ResponseWriter, r *http.Request) {
		poolIDStr := r.URL.Query().Get("pool_id")
		if poolIDStr == "" {
			leases, err := store.ListActiveLeases(r.Context())
			if err != nil {
				failure(w, 503, "storage_unavailable", "Leases unavailable")
				return
			}
			respond(w, 200, leases)
			return
		}
		poolID, err := strconv.ParseInt(poolIDStr, 10, 64)
		if err != nil {
			failure(w, 400, "invalid_pool_id", "Invalid pool ID")
			return
		}
		leases, err := store.ListLeases(r.Context(), poolID)
		if err != nil {
			failure(w, 503, "storage_unavailable", "Leases unavailable")
			return
		}
		respond(w, 200, leases)
	})

	mux.HandleFunc("DELETE /api/v1/dhcp/leases/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			failure(w, 400, "invalid_id", "Invalid lease ID")
			return
		}
		if err := store.DeleteLease(r.Context(), id); err != nil {
			failure(w, 500, "storage_error", "Failed to delete lease")
			return
		}
		respond(w, 204, nil)
	})
}
