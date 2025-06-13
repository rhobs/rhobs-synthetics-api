package main

import (
	"log"
	"net/http"
	"os"

	"github.com/rhobs/rhobs-synthetics/pkg/api"
)

// TODO: Move the webserver logic into it's own func and call it based on cobra startup args
func main() {

	log.SetOutput(os.Stdout)

	log.Println("Application starting up...")
	server := api.NewServer()

	r := http.NewServeMux()

	h := api.HandlerFromMux(server, r)

	s := &http.Server{
		Handler: h,
		Addr:    "0.0.0.0:8080",
	}

	log.Fatal(s.ListenAndServe())
}
