package handler

import (
	"context"

	"github.com/hi-lee-mon/copan/apps/api/internal/rest/gen"
)

type Healthz struct{}

func (Healthz) HealthCheck(ctx context.Context, request gen.HealthCheckRequestObject) (gen.HealthCheckResponseObject, error) {
	// Version は設定しない。契約上は省略可能で、いま返せる値ではデプロイ済みの版を判別できないため
	return gen.HealthCheck200JSONResponse{Status: gen.OK}, nil
}
