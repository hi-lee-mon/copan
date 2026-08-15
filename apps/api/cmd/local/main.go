package main

import (
	"log"
	"net/http"

	"github.com/hi-lee-mon/copan/apps/api/internal/health/interface/rest/handler"
)

func main() {
	router := handler.NewRouter()
	if err := http.ListenAndServe(":8081", router); err != nil {
		log.Fatal(err)
	}
}
