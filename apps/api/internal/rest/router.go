package rest

import (
	"net/http"

	"github.com/hi-lee-mon/copan/apps/api/internal/health/interface/rest/handler"
)

func NewRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /ping", handler.Ping)

	return mux
}
