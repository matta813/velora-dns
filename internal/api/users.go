package api

import (
	"net/http"
	"strconv"

	"github.com/matta813/velora-dns/internal/auth"
)

func registerUsers(mux *http.ServeMux, service *auth.Service) {
	mux.HandleFunc("GET /api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		users, err := service.Store().ListUsers(r.Context())
		if err != nil {
			authFailure(w, err)
			return
		}
		respond(w, 200, users)
	})
	mux.HandleFunc("POST /api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Username string    `json:"username"`
			Password string    `json:"password"`
			Role     auth.Role `json:"role"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		if err := auth.ValidateUsername(input.Username); err != nil {
			failure(w, 400, "invalid_user", err.Error())
			return
		}
		if err := auth.ValidatePassword(input.Password); err != nil {
			failure(w, 400, "invalid_user", err.Error())
			return
		}
		if !auth.ValidateRole(input.Role) {
			failure(w, 400, "invalid_user", "Role must be admin, operator or viewer")
			return
		}
		hash, err := auth.HashPassword(input.Password)
		if err != nil {
			authFailure(w, err)
			return
		}
		user, err := service.Store().CreateUser(r.Context(), input.Username, hash, input.Role)
		if err != nil {
			authFailure(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/users/"+strconv.FormatInt(user.ID, 10))
		respond(w, 201, user)
	})
	mux.HandleFunc("PUT /api/v1/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := auth.ParseID(r.PathValue("id"))
		if err != nil {
			authFailure(w, err)
			return
		}
		var input struct {
			Password *string    `json:"password"`
			Role     *auth.Role `json:"role"`
			Active   *bool      `json:"active"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		if input.Password == nil && input.Role == nil && input.Active == nil {
			failure(w, 400, "invalid_user", "At least one change is required")
			return
		}
		var hash *string
		if input.Password != nil {
			if err = auth.ValidatePassword(*input.Password); err != nil {
				failure(w, 400, "invalid_user", err.Error())
				return
			}
			value, hashErr := auth.HashPassword(*input.Password)
			if hashErr != nil {
				authFailure(w, hashErr)
				return
			}
			hash = &value
		}
		if input.Role != nil && !auth.ValidateRole(*input.Role) {
			failure(w, 400, "invalid_user", "Role must be admin, operator or viewer")
			return
		}
		user, err := service.Store().UpdateUser(r.Context(), id, hash, input.Role, input.Active)
		if err != nil {
			authFailure(w, err)
			return
		}
		respond(w, 200, user)
	})
	mux.HandleFunc("DELETE /api/v1/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := auth.ParseID(r.PathValue("id"))
		if err != nil {
			authFailure(w, err)
			return
		}
		if err = service.Store().DeleteUser(r.Context(), id); err != nil {
			authFailure(w, err)
			return
		}
		respond(w, 200, map[string]int64{"deleted": id})
	})
	mux.HandleFunc("GET /api/v1/audit", func(w http.ResponseWriter, r *http.Request) {
		limit := 100
		if value := r.URL.Query().Get("limit"); value != "" {
			var err error
			limit, err = strconv.Atoi(value)
			if err != nil || limit < 1 || limit > 500 {
				failure(w, 400, "invalid_limit", "Limit must be 1–500")
				return
			}
		}
		events, err := service.Store().ListAudit(r.Context(), limit)
		if err != nil {
			authFailure(w, err)
			return
		}
		respond(w, 200, events)
	})
	for path, methods := range map[string]string{"/api/v1/users": "GET, HEAD, POST", "/api/v1/users/{id}": "PUT, DELETE", "/api/v1/audit": "GET, HEAD", "/api/v1/auth/login": "POST", "/api/v1/auth/logout": "POST", "/api/v1/auth/session": "GET, HEAD"} {
		mux.HandleFunc(path, methodError(methods))
	}
}
