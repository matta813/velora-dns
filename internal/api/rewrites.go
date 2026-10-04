package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/matta813/velora-dns/internal/rewrites"
)

type RewriteStore interface {
	List(context.Context) ([]rewrites.RuleStatus, error)
	Create(context.Context, rewrites.Rule) (rewrites.Rule, error)
	Update(context.Context, int64, rewrites.Rule) (rewrites.Rule, error)
	Delete(context.Context, int64) error
}

func rewriteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, rewrites.ErrNotFound):
		failure(w, 404, "not_found", "Rewrite not found")
	case errors.Is(err, rewrites.ErrExists):
		failure(w, 409, "rewrite_exists", err.Error())
	case errors.Is(err, rewrites.ErrConflict):
		failure(w, 409, "rewrite_conflict", err.Error())
	case errors.Is(err, rewrites.ErrInvalid):
		failure(w, 400, "invalid_rewrite", err.Error())
	default:
		failure(w, 503, "rewrites_unavailable", "Rewrites could not be saved; the previous rewrites remain active")
	}
}

type rewriteInput struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Value       string `json:"value"`
	Enabled     *bool  `json:"enabled"`
	Description string `json:"description"`
}

func (in rewriteInput) rule() rewrites.Rule {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return rewrites.Rule{Name: in.Name, Type: in.Type, Value: in.Value, Enabled: enabled, Description: in.Description}
}

func registerRewrites(mux *http.ServeMux, service RewriteStore) {
	id := func(w http.ResponseWriter, r *http.Request) (int64, bool) {
		value, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || value < 1 {
			failure(w, 400, "invalid_id", "Invalid rewrite ID")
			return 0, false
		}
		return value, true
	}
	mux.HandleFunc("GET /api/v1/rewrites", func(w http.ResponseWriter, r *http.Request) {
		rules, err := service.List(r.Context())
		if err != nil {
			rewriteError(w, err)
			return
		}
		respond(w, 200, rules)
	})
	mux.HandleFunc("POST /api/v1/rewrites", func(w http.ResponseWriter, r *http.Request) {
		var input rewriteInput
		if !readJSON(w, r, &input) {
			return
		}
		rule, err := service.Create(r.Context(), input.rule())
		if err != nil {
			rewriteError(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/rewrites/"+strconv.FormatInt(rule.ID, 10))
		respond(w, 201, rule)
	})
	mux.HandleFunc("PUT /api/v1/rewrites/{id}", func(w http.ResponseWriter, r *http.Request) {
		ruleID, ok := id(w, r)
		if !ok {
			return
		}
		var input rewriteInput
		if !readJSON(w, r, &input) {
			return
		}
		rule, err := service.Update(r.Context(), ruleID, input.rule())
		if err != nil {
			rewriteError(w, err)
			return
		}
		respond(w, 200, rule)
	})
	mux.HandleFunc("DELETE /api/v1/rewrites/{id}", func(w http.ResponseWriter, r *http.Request) {
		ruleID, ok := id(w, r)
		if !ok {
			return
		}
		if err := service.Delete(r.Context(), ruleID); err != nil {
			rewriteError(w, err)
			return
		}
		respond(w, 200, map[string]int64{"deleted": ruleID})
	})
}
