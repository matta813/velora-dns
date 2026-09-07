package api

import (
	"github.com/matta813/velora-dns/internal/database"
	"net/http"
	"strconv"
)

func registerQueries(mux *http.ServeMux, store QueryStore) {
	mux.HandleFunc("GET /api/v1/queries", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		entries, err := store.ListQueries(r.Context(), database.QueryFilter{Domain: q.Get("domain"), Client: q.Get("client"), Type: q.Get("type"), Source: q.Get("source"), Limit: limit})
		if err != nil {
			failure(w, 503, "storage_unavailable", "Query log storage is unavailable")
			return
		}
		respond(w, 200, entries)
	})
}
