package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/matta813/velora-dns/internal/update"
)

type UpdateStore interface {
	Status(context.Context) (update.Status, error)
	History(context.Context) ([]update.Entry, error)
	Check(context.Context) (update.CheckResult, error)
	Request(context.Context) (update.RequestResponse, error)
}

func registerUpdate(mux *http.ServeMux, store UpdateStore, _ Version) {
	mux.HandleFunc("GET /api/v1/update/status", func(w http.ResponseWriter, r *http.Request) {
		status, err := store.Status(r.Context())
		if err != nil {
			updaterFailure(w, err)
			return
		}
		respond(w, http.StatusOK, status)
	})
	mux.HandleFunc("GET /api/v1/update/history", func(w http.ResponseWriter, r *http.Request) {
		history, err := store.History(r.Context())
		if err != nil {
			updaterFailure(w, err)
			return
		}
		respond(w, http.StatusOK, history)
	})
	mux.HandleFunc("GET /api/v1/update/check", func(w http.ResponseWriter, r *http.Request) {
		result, err := store.Check(r.Context())
		if err != nil {
			updaterFailure(w, err)
			return
		}
		respond(w, http.StatusOK, result)
	})
	mux.HandleFunc("POST /api/v1/update/request", func(w http.ResponseWriter, r *http.Request) {
		type updateRequest struct {
			Action string `json:"action"`
		}
		var req updateRequest
		if !readJSON(w, r, &req) {
			return
		}
		if req.Action != "update" {
			failure(w, http.StatusBadRequest, "invalid_action", "Only 'update' action is supported")
			return
		}
		result, err := store.Request(r.Context())
		if err != nil {
			updaterFailure(w, err)
			return
		}
		respond(w, http.StatusAccepted, result)
	})
}

func updaterFailure(w http.ResponseWriter, err error) {
	var agentErr *update.AgentError
	if errors.As(err, &agentErr) {
		code := "updater_rejected"
		if agentErr.StatusCode == http.StatusConflict {
			code = "update_in_progress"
		}
		failure(w, agentErr.StatusCode, code, agentErr.Message)
		return
	}
	failure(w, http.StatusServiceUnavailable, "updater_unavailable", err.Error())
}
