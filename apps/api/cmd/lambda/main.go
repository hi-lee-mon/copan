package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/hi-lee-mon/copan/apps/api/internal/health/interface/rest/handler"
)

func main() {
	router := handler.NewRouter()
	adapter := httpadapter.NewV2(router)
	lambda.Start(adapter.ProxyWithContext)
}
