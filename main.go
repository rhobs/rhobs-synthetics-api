package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	middleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/rhobs/rhobs-synthetics/pkg/api"
	"github.com/spf13/cobra"
)

// runWebServer starts the HTTP server.
func runWebServer(addr string) error {
	log.Printf("Starting web server on %s...", addr)

	swagger, err := api.GetSwagger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading swagger spec\n: %s", err)
	}

	swagger.Servers = nil

	server := api.NewServer()

	r := http.NewServeMux()

	api.HandlerFromMux(server, r)

	// Use validation middleware to check all requests against the OpenAPI schema.
	h := middleware.OapiRequestValidator(swagger)(r)

	s := &http.Server{
		Handler: h,
		Addr:    addr,
	}

	return s.ListenAndServe()
}

func main() {

	log.SetOutput(os.Stdout)
	log.Println("Application starting up...")

	var listenAddr string

	// rootCmd represents the base command when called without any subcommands
	var rootCmd = &cobra.Command{
		Use:   "rhobs-synthetics",
		Short: "RHOBS Synthetics Monitoring API/Agent.",
		Long:  `This application provides a synthetic monitoring API and Agent to be used within the RHOBS ecosystem.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	// apiCmd represents the 'api' subcommand
	var apiCmd = &cobra.Command{
		Use:   "api",
		Short: "Start the API web server",
		Long:  `Starts the HTTP server to expose the synthetics API.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := runWebServer(listenAddr); err != nil {
				log.Fatalf("Web server failed: %v", err)
			}
		},
	}

	// Add flags for the serve command
	apiCmd.Flags().StringVarP(&listenAddr, "listen-addr", "l", "0.0.0.0:8080", "Address to listen on for HTTP requests")

	// Add commands to the root command
	rootCmd.AddCommand(apiCmd)

	// Execute the root command. This parses the arguments and calls the appropriate command's Run function.
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
