package main

import (
	"log"
	"net/http"

	"github.com/hi-lee-mon/copan/apps/api/internal/rest"
)

func main() {
	router := rest.NewRouter()
	if err := http.ListenAndServe(":8081", router); err != nil {
		log.Fatal(err)
	}
}
