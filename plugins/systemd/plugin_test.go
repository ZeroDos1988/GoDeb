package systemd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRejectsOwnUnit(t *testing.T) {
	mux := http.NewServeMux()
	Mount(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/plugins/systemd/action", strings.NewReader(`{"unit":"godeb.service","action":"stop"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}
