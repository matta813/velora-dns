package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func web(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			failure(w, 405, "method_not_allowed", "Method not allowed")
			return
		}
		// Only explicit UI routes use the SPA fallback; missing assets remain 404.
		switch r.URL.Path {
		case "/", "/cache", "/settings":
			if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
				failure(w, 503, "ui_unavailable", "Build the web UI with npm run build")
				return
			}
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
		default:
			if !strings.HasPrefix(r.URL.Path, "/assets/") && r.URL.Path != "/favicon.svg" {
				failure(w, 404, "not_found", "Page not found")
				return
			}
			fs.ServeHTTP(w, r)
		}
	})
}
