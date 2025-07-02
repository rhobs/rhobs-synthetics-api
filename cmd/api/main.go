package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	middleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/rhobs/rhobs-synthetics-api/internal/api"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// runWebServer starts the HTTP server.
func runWebServer(addr string) error {

	swagger, err := v1.GetSwagger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading swagger spec\n: %s", err)
	}

	swagger.Servers = nil

	server := api.NewServer()

	serverHandler := v1.NewStrictHandler(server, nil)

	r := http.NewServeMux()

	v1.HandlerFromMux(serverHandler, r)

	// Use validation middleware to check all requests against the OpenAPI schema.
	h := middleware.OapiRequestValidator(swagger)(r)

	s := &http.Server{
		Handler:      h,
		Addr:         addr,
		ReadTimeout:  viper.GetDuration("read_timeout"),
		WriteTimeout: viper.GetDuration("write_timeout"),
	}

	log.Printf("Listening on http://%s", addr)
	return s.ListenAndServe()
}

func main() {

	log.SetOutput(os.Stdout)
	log.Println("Application starting up...")

	// rootCmd represents the base command when called without any subcommands
	var rootCmd = &cobra.Command{
		Use:   "rhobs-synthetics",
		Short: "RHOBS Synthetics Monitoring API/Agent.",
		Long:  `This application provides a synthetic monitoring API and Agent to be used within the RHOBS ecosystem.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			configPath := viper.GetString("config")
			if configPath != "" {
				viper.SetConfigFile(configPath)
				if err := viper.ReadInConfig(); err != nil {
					return fmt.Errorf("failed to read config: %w", err)
				}
			}
			return nil
		},
	}

	// apiCmd represents the 'start' subcommand
	var startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the API web server",
		Long:  `Starts the HTTP server to expose the synthetics API.`,
		Run: func(cmd *cobra.Command, args []string) {
			host := viper.GetString("host")
			port := viper.GetInt("port")
			listenAddr := fmt.Sprintf("%s:%d", host, port)

			if err := runWebServer(listenAddr); err != nil {
				log.Fatalf("Web server failed: %v", err)
			}
		},
	}

	// Add flags for the serve command

	// API Server flags
	startCmd.Flags().IntP("port", "p", 8080, "Port to run the server on (e.g., 8080)")
	startCmd.Flags().String("host", "0.0.0.0", "Host address to bind")
	startCmd.Flags().Duration("read-timeout", 5*time.Second, "Max duration for reading the entire request (e.g. 5s)")
	startCmd.Flags().Duration("write-timeout", 10*time.Second, "Max duration before timing out writes")
	startCmd.Flags().Duration("graceful-timeout", 15*time.Second, "Time allowed for graceful shutdown")
	startCmd.Flags().String("database-engine", "etcd", "Specifies the backend database engine for persisting probe configurations (default: etcd)")

	// General Config flags
	startCmd.Flags().String("config", "", "Path to Viper config")
	startCmd.Flags().String("log-level", "info", "Log verbosity: debug, info")

	// Bind flags to viper
	viper.BindPFlag("port", startCmd.Flags().Lookup("port"))
	viper.BindPFlag("host", startCmd.Flags().Lookup("host"))
	viper.BindPFlag("read_timeout", startCmd.Flags().Lookup("read-timeout"))
	viper.BindPFlag("write_timeout", startCmd.Flags().Lookup("write-timeout"))
	viper.BindPFlag("graceful_timeout", startCmd.Flags().Lookup("graceful-timeout"))
	viper.BindPFlag("database_engine", startCmd.Flags().Lookup("database-engine"))
	viper.BindPFlag("config", startCmd.Flags().Lookup("config"))
	viper.BindPFlag("log_level", startCmd.Flags().Lookup("log-level"))

	// Add commands to the root command
	rootCmd.AddCommand(startCmd)

	// Execute the root command. This parses the arguments and calls the appropriate command's Run function.
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
