package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/matta813/velora-dns/internal/forwarding"
	wire "github.com/miekg/dns"
)

type ForwardingStore interface {
	List(context.Context) ([]forwarding.RuleStatus, error)
	Create(context.Context, forwarding.Rule) (forwarding.Rule, error)
	Update(context.Context, int64, forwarding.Rule) (forwarding.Rule, error)
	Delete(context.Context, int64) error
	Test(context.Context, int64, string, uint16) (forwarding.TestResult, error)
}

func forwardingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, forwarding.ErrNotFound):
		failure(w, 404, "not_found", "Forwarding rule not found")
	case errors.Is(err, forwarding.ErrExists):
		failure(w, 409, "rule_exists", err.Error())
	case errors.Is(err, forwarding.ErrInvalid):
		failure(w, 400, "invalid_rule", err.Error())
	default:
		failure(w, 503, "forwarding_unavailable", "Forwarding rules could not be saved; the previous rules remain active")
	}
}

func forwardingID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		failure(w, 400, "invalid_id", "Invalid rule ID")
		return 0, false
	}
	return id, true
}

type forwardingInput struct {
	Domain      string   `json:"domain"`
	Upstreams   []string `json:"upstreams"`
	Enabled     *bool    `json:"enabled"`
	Description string   `json:"description"`
}

func (in forwardingInput) rule() forwarding.Rule {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return forwarding.Rule{Domain: in.Domain, Upstreams: in.Upstreams, Enabled: enabled, Description: in.Description}
}

func registerForwarding(mux *http.ServeMux, service ForwardingStore) {
	mux.HandleFunc("GET /api/v1/forwarding", func(w http.ResponseWriter, r *http.Request) {
		rules, err := service.List(r.Context())
		if err != nil {
			forwardingError(w, err)
			return
		}
		respond(w, 200, rules)
	})
	mux.HandleFunc("POST /api/v1/forwarding", func(w http.ResponseWriter, r *http.Request) {
		var input forwardingInput
		if !readJSON(w, r, &input) {
			return
		}
		rule, err := service.Create(r.Context(), input.rule())
		if err != nil {
			forwardingError(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/forwarding/"+strconv.FormatInt(rule.ID, 10))
		respond(w, 201, rule)
	})
	mux.HandleFunc("PUT /api/v1/forwarding/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := forwardingID(w, r)
		if !ok {
			return
		}
		var input forwardingInput
		if !readJSON(w, r, &input) {
			return
		}
		rule, err := service.Update(r.Context(), id, input.rule())
		if err != nil {
			forwardingError(w, err)
			return
		}
		respond(w, 200, rule)
	})
	mux.HandleFunc("DELETE /api/v1/forwarding/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := forwardingID(w, r)
		if !ok {
			return
		}
		if err := service.Delete(r.Context(), id); err != nil {
			forwardingError(w, err)
			return
		}
		respond(w, 200, map[string]int64{"deleted": id})
	})
	mux.HandleFunc("POST /api/v1/forwarding/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		id, ok := forwardingID(w, r)
		if !ok {
			return
		}
		var input struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		var qtype uint16
		if raw := strings.ToUpper(strings.TrimSpace(input.Type)); raw != "" {
			value, known := wire.StringToType[raw]
			if !known {
				failure(w, 400, "invalid_type", "Unknown record type")
				return
			}
			qtype = value
		}
		result, err := service.Test(r.Context(), id, input.Name, qtype)
		if err != nil {
			forwardingError(w, err)
			return
		}
		respond(w, 200, result)
	})
}
