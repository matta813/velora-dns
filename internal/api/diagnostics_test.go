package api

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiagnosticsReportsRealHealthWithoutSecrets(t *testing.T) {
	for _, tc := range []struct {
		name string
		db   fakeDB
		want string
	}{
		{name: "healthy", db: fakeDB{}, want: "healthy"},
		{name: "storage unavailable", db: fakeDB{err: errors.New("password=secret database failure")}, want: "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler(tc.db, true).ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1/api/v1/diagnostics", nil))
			if w.Code != 200 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			var body struct {
				Data DiagnosticReport `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Data.State != tc.want {
				t.Fatalf("report state: %+v (%v)", body.Data, err)
			}
			if strings.Contains(w.Body.String(), "password=secret") || strings.Contains(w.Body.String(), "database_path") {
				t.Fatalf("report leaked sensitive data: %s", w.Body.String())
			}
		})
	}
}
