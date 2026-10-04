package api

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/matta813/velora-dns/internal/clients"
	"github.com/matta813/velora-dns/internal/querylog"
)

type ClientStore interface {
	List(context.Context) ([]clients.Client, error)
	Create(context.Context, clients.Client) (clients.Client, error)
	Update(context.Context, int64, clients.Client) (clients.Client, error)
	Delete(context.Context, int64) error
	Lookup(netip.Addr) (clients.Client, bool)
	Name(string) string
}

type ActivityStore interface {
	ClientActivity(context.Context, time.Time, int) ([]querylog.ClientActivity, error)
}

type clientActivity struct {
	Queries  uint64     `json:"queries"`
	LastSeen *time.Time `json:"last_seen"`
}

type clientView struct {
	clients.Client
	// Activity covers the last 24 hours of retained queries; it is null when
	// query logging is disabled.
	Activity *clientActivity `json:"activity"`
}

type clientList struct {
	Clients []clientView `json:"clients"`
	// Unnamed lists busy addresses seen recently that match no client.
	Unnamed []querylog.ClientActivity `json:"unnamed"`
}

func clientError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, clients.ErrNotFound):
		failure(w, 404, "not_found", "Client not found")
	case errors.Is(err, clients.ErrExists):
		failure(w, 409, "client_exists", err.Error())
	case errors.Is(err, clients.ErrInvalid):
		failure(w, 400, "invalid_client", err.Error())
	default:
		failure(w, 503, "clients_unavailable", "Clients could not be saved")
	}
}

type clientInput struct {
	Name        string   `json:"name"`
	Addresses   []string `json:"addresses"`
	Group       string   `json:"group"`
	Description string   `json:"description"`
	Enabled     *bool    `json:"enabled"`
}

func (in clientInput) client() clients.Client {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return clients.Client{Name: in.Name, Addresses: in.Addresses, Group: in.Group, Description: in.Description, Enabled: enabled}
}

func registerClients(mux *http.ServeMux, service ClientStore, activity ActivityStore, queryLogging func() bool) {
	mux.HandleFunc("GET /api/v1/clients", func(w http.ResponseWriter, r *http.Request) {
		list, err := service.List(r.Context())
		if err != nil {
			clientError(w, err)
			return
		}
		result := clientList{Clients: make([]clientView, 0, len(list)), Unnamed: []querylog.ClientActivity{}}
		byID := map[int64]*clientActivity{}
		if activity != nil && queryLogging() {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			seen, err := activity.ClientActivity(ctx, time.Now().Add(-24*time.Hour), 5000)
			cancel()
			if err == nil {
				for _, client := range list {
					byID[client.ID] = &clientActivity{}
				}
				for _, item := range seen {
					addr, parseErr := netip.ParseAddr(item.ClientIP)
					if parseErr != nil {
						continue
					}
					owner, ok := service.Lookup(addr)
					if !ok {
						if len(result.Unnamed) < 20 {
							result.Unnamed = append(result.Unnamed, item)
						}
						continue
					}
					summary := byID[owner.ID]
					summary.Queries += item.Queries
					if last := item.LastSeen; summary.LastSeen == nil || last.After(*summary.LastSeen) {
						summary.LastSeen = &last
					}
				}
			}
		}
		for _, client := range list {
			result.Clients = append(result.Clients, clientView{Client: client, Activity: byID[client.ID]})
		}
		respond(w, 200, result)
	})
	mux.HandleFunc("POST /api/v1/clients", func(w http.ResponseWriter, r *http.Request) {
		var input clientInput
		if !readJSON(w, r, &input) {
			return
		}
		client, err := service.Create(r.Context(), input.client())
		if err != nil {
			clientError(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/clients/"+strconv.FormatInt(client.ID, 10))
		respond(w, 201, client)
	})
	mux.HandleFunc("PUT /api/v1/clients/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			failure(w, 400, "invalid_id", "Invalid client ID")
			return
		}
		var input clientInput
		if !readJSON(w, r, &input) {
			return
		}
		client, err := service.Update(r.Context(), id, input.client())
		if err != nil {
			clientError(w, err)
			return
		}
		respond(w, 200, client)
	})
	mux.HandleFunc("DELETE /api/v1/clients/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			failure(w, 400, "invalid_id", "Invalid client ID")
			return
		}
		if err := service.Delete(r.Context(), id); err != nil {
			clientError(w, err)
			return
		}
		respond(w, 200, map[string]int64{"deleted": id})
	})
}
