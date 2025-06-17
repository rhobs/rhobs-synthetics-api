package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	middleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/rhobs/rhobs-synthetics/pkg/api"
)

// TODO: Move the webserver logic into it's own func and call it based on cobra startup args
func main() {

	log.SetOutput(os.Stdout)

	log.Println("Application starting up...")

	swagger, err := api.GetSwagger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading swagger spec\n: %s", err)
		os.Exit(1)
	}

	swagger.Servers = nil

	server := api.NewServer()

	r := http.NewServeMux()

	api.HandlerFromMux(server, r)

	// Use validation middleware to check all requests against the
	// OpenAPI schema.
	h := middleware.OapiRequestValidator(swagger)(r)

	s := &http.Server{
		Handler: h,
		Addr:    "0.0.0.0:8080",
	}

	log.Fatal(s.ListenAndServe())
}
