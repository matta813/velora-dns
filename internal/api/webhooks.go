package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/matta813/velora-dns/internal/webhooks"
)

type WebhookStore interface {
	List(context.Context) ([]webhooks.Hook, error)
	Create(context.Context, webhooks.Hook) (webhooks.Hook, error)
	Update(context.Context, int64, webhooks.Hook, bool) (webhooks.Hook, error)
	Delete(context.Context, int64) error
	Test(context.Context, int64) (webhooks.Hook, error)
}

func webhookError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, webhooks.ErrNotFound):
		failure(w, 404, "not_found", "Webhook not found")
	case errors.Is(err, webhooks.ErrExists):
		failure(w, 409, "webhook_exists", err.Error())
	case errors.Is(err, webhooks.ErrInvalid):
		failure(w, 400, "invalid_webhook", err.Error())
	default:
		failure(w, 503, "webhooks_unavailable", "Webhooks could not be saved")
	}
}

type webhookInput struct {
	Name         string   `json:"name"`
	URL          string   `json:"url"`
	Events       []string `json:"events"`
	MinSeverity  string   `json:"min_severity"`
	AllowPrivate bool     `json:"allow_private"`
	Enabled      *bool    `json:"enabled"`
	// Token is write-only. On update, null keeps the stored token and ""
	// removes it.
	Token *string `json:"token"`
}

func (in webhookInput) hook() webhooks.Hook {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	hook := webhooks.Hook{Name: in.Name, URL: in.URL, Events: in.Events, MinSeverity: in.MinSeverity, AllowPrivate: in.AllowPrivate, Enabled: enabled}
	if in.Token != nil {
		hook.Token = *in.Token
	}
	return hook
}

type webhookTestResult struct {
	OK      bool          `json:"ok"`
	Error   string        `json:"error"`
	Webhook webhooks.Hook `json:"webhook"`
}

func registerWebhooks(mux *http.ServeMux, service WebhookStore) {
	mux.HandleFunc("GET /api/v1/webhooks", func(w http.ResponseWriter, r *http.Request) {
		list, err := service.List(r.Context())
		if err != nil {
			webhookError(w, err)
			return
		}
		respond(w, 200, list)
	})
	mux.HandleFunc("GET /api/v1/webhooks/event-types", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, webhooks.EventTypes)
	})
	mux.HandleFunc("POST /api/v1/webhooks", func(w http.ResponseWriter, r *http.Request) {
		var input webhookInput
		if !readJSON(w, r, &input) {
			return
		}
		hook, err := service.Create(r.Context(), input.hook())
		if err != nil {
			webhookError(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/webhooks/"+strconv.FormatInt(hook.ID, 10))
		respond(w, 201, hook)
	})
	id := func(w http.ResponseWriter, r *http.Request) (int64, bool) {
		value, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || value < 1 {
			failure(w, 400, "invalid_id", "Invalid webhook ID")
			return 0, false
		}
		return value, true
	}
	mux.HandleFunc("PUT /api/v1/webhooks/{id}", func(w http.ResponseWriter, r *http.Request) {
		hookID, ok := id(w, r)
		if !ok {
			return
		}
		var input webhookInput
		if !readJSON(w, r, &input) {
			return
		}
		hook, err := service.Update(r.Context(), hookID, input.hook(), input.Token == nil)
		if err != nil {
			webhookError(w, err)
			return
		}
		respond(w, 200, hook)
	})
	mux.HandleFunc("DELETE /api/v1/webhooks/{id}", func(w http.ResponseWriter, r *http.Request) {
		hookID, ok := id(w, r)
		if !ok {
			return
		}
		if err := service.Delete(r.Context(), hookID); err != nil {
			webhookError(w, err)
			return
		}
		respond(w, 200, map[string]int64{"deleted": hookID})
	})
	mux.HandleFunc("POST /api/v1/webhooks/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		hookID, ok := id(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		hook, err := service.Test(ctx, hookID)
		if errors.Is(err, webhooks.ErrNotFound) {
			webhookError(w, err)
			return
		}
		result := webhookTestResult{OK: err == nil, Webhook: hook}
		if err != nil {
			result.Error = err.Error()
		}
		respond(w, 200, result)
	})
}
