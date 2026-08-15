package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hi-lee-mon/copan/apps/api/internal/health/interface/rest/handler"
)

func TestPing(t *testing.T) {
	h := handler.NewRouter()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
