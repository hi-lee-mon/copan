package rest

import (
	"github.com/hi-lee-mon/copan/apps/api/internal/healthz/interface/rest/handler"
)

// 各モジュールのハンドラを埋め込み、gen.StrictServerInterface を満たす
type server struct {
	handler.Healthz
}
