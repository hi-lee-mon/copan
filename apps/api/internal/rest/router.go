package rest

import (
	"net/http"

	"github.com/hi-lee-mon/copan/apps/api/internal/rest/gen"
)

func NewRouter() http.Handler {
	si := gen.NewStrictHandler(server{}, nil)

	return gen.HandlerWithOptions(si, gen.StdHTTPServerOptions{})
}
