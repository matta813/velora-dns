package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strconv"

	"github.com/matta813/velora-dns/internal/clients"
	"github.com/matta813/velora-dns/internal/filtering"
	"github.com/matta813/velora-dns/internal/policies"
)

type PolicyStore interface {
	List(context.Context) ([]policies.Policy, error)
	Create(context.Context, policies.Policy) (policies.Policy, error)
	Update(context.Context, int64, policies.Policy) (policies.Policy, error)
	Delete(context.Context, int64) error
	Effective(netip.Addr) policies.Effective
}

func policyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, policies.ErrNotFound):
		failure(w, 404, "not_found", "Policy not found")
	case errors.Is(err, policies.ErrExists):
		failure(w, 409, "policy_exists", err.Error())
	case errors.Is(err, policies.ErrInvalid):
		failure(w, 400, "invalid_policy", err.Error())
	default:
		failure(w, 503, "policies_unavailable", "Policies could not be saved")
	}
}

type policyInput struct {
	ClientID   int64    `json:"client_id"`
	Mode       string   `json:"mode"`
	Blocklists []int64  `json:"blocklists"`
	Allow      []string `json:"allow"`
	Block      []string `json:"block"`
	Enabled    *bool    `json:"enabled"`
}

func (in policyInput) policy() policies.Policy {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return policies.Policy{ClientID: in.ClientID, Mode: in.Mode, Blocklists: in.Blocklists, Allow: in.Allow, Block: in.Block, Enabled: enabled}
}

// validatePolicyRefs rejects policies for unknown clients or blocklists, so a
// typo cannot silently turn into "no filtering".
func validatePolicyRefs(ctx context.Context, in policyInput, known ClientStore, lists BlocklistStore) error {
	if known != nil {
		all, err := known.List(ctx)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(all, func(client clients.Client) bool { return client.ID == in.ClientID }) {
			return fmt.Errorf("%w: unknown client %d", policies.ErrInvalid, in.ClientID)
		}
	}
	if lists != nil && in.Mode == policies.ModeCustom {
		sources, err := lists.List(ctx)
		if err != nil {
			return err
		}
		for _, id := range in.Blocklists {
			if !slices.ContainsFunc(sources, func(source filtering.Source) bool { return source.ID == id }) {
				return fmt.Errorf("%w: unknown blocklist %d", policies.ErrInvalid, id)
			}
		}
	}
	return nil
}

func registerPolicies(mux *http.ServeMux, service PolicyStore, known ClientStore, lists BlocklistStore) {
	mux.HandleFunc("GET /api/v1/policies", func(w http.ResponseWriter, r *http.Request) {
		list, err := service.List(r.Context())
		if err != nil {
			policyError(w, err)
			return
		}
		respond(w, 200, list)
	})
	mux.HandleFunc("GET /api/v1/policies/effective", func(w http.ResponseWriter, r *http.Request) {
		addr, err := netip.ParseAddr(r.URL.Query().Get("ip"))
		if err != nil || addr.Zone() != "" {
			failure(w, 400, "invalid_ip", "Enter a valid IP address")
			return
		}
		respond(w, 200, service.Effective(addr.Unmap()))
	})
	save := func(w http.ResponseWriter, r *http.Request, id int64) {
		var input policyInput
		if !readJSON(w, r, &input) {
			return
		}
		if err := validatePolicyRefs(r.Context(), input, known, lists); err != nil {
			policyError(w, err)
			return
		}
		var (
			policy policies.Policy
			err    error
		)
		if id == 0 {
			policy, err = service.Create(r.Context(), input.policy())
		} else {
			policy, err = service.Update(r.Context(), id, input.policy())
		}
		if err != nil {
			policyError(w, err)
			return
		}
		if id == 0 {
			w.Header().Set("Location", "/api/v1/policies/"+strconv.FormatInt(policy.ID, 10))
			respond(w, 201, policy)
			return
		}
		respond(w, 200, policy)
	}
	mux.HandleFunc("POST /api/v1/policies", func(w http.ResponseWriter, r *http.Request) { save(w, r, 0) })
	mux.HandleFunc("PUT /api/v1/policies/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			failure(w, 400, "invalid_id", "Invalid policy ID")
			return
		}
		save(w, r, id)
	})
	mux.HandleFunc("DELETE /api/v1/policies/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			failure(w, 400, "invalid_id", "Invalid policy ID")
			return
		}
		if err := service.Delete(r.Context(), id); err != nil {
			policyError(w, err)
			return
		}
		respond(w, 200, map[string]int64{"deleted": id})
	})
}
