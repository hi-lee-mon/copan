package rest_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hi-lee-mon/copan/apps/api/internal/rest"
	"github.com/hi-lee-mon/copan/apps/api/internal/rest/gen"
)

func TestHealthz(t *testing.T) {
	h := rest.NewRouter()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}

	var got gen.HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if got.Status != gen.OK {
		t.Errorf("expected OK, got %q", got.Status)
	}
	if got.Version != nil {
		t.Errorf("expected version to be omitted, got %q", *got.Version)
	}
}

func TestPingIsRemoved(t *testing.T) {
	h := rest.NewRouter()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}
