package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	middleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/rhobs/rhobs-synthetics-api/internal/api"
	"github.com/rhobs/rhobs-synthetics-api/internal/probestore"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
	"github.com/rhobs/rhobs-synthetics-api/web"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// runWebServer starts the HTTP server.
func runWebServer(addr string) error {

	swagger, err := v1.GetSwagger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading swagger spec\n: %s", err)
	}

	swagger.Servers = nil

	// Try to create in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		// If in-cluster fails, try to use kubeconfig
		log.Printf("Could not create in-cluster config: %v. Trying to use kubeconfig.", err)
		kubeconfigPath := viper.GetString("kubeconfig")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return fmt.Errorf("failed to create kubernetes client config from kubeconfig: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	namespace := viper.GetString("namespace")

	store, err := probestore.NewKubernetesProbeStore(context.Background(), clientset, namespace)
	if err != nil {
		return fmt.Errorf("failed to create probe store: %w", err)
	}
	server := api.NewServer(store)
	serverHandler := v1.NewStrictHandler(server, nil)

	// The main router
	mux := http.NewServeMux()

	// Add the Swagger UI handler at /docs
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(web.SwaggerHTML)
	})

	// Add the OpenAPI spec handler at /api/v1/openapi.json
	mux.HandleFunc("/api/v1/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		jsonSpec, err := swagger.MarshalJSON()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to marshal swagger spec: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonSpec)
	})

	// The API handlers are registered on a separate router and validated.
	apiRouter := http.NewServeMux()
	v1.HandlerFromMux(serverHandler, apiRouter)
	validatedAPI := middleware.OapiRequestValidator(swagger)(apiRouter)

	// Mount the validated API router to the main router.
	// Requests will be matched against the UI handlers first, then fall through to the API.
	mux.Handle("/", validatedAPI)

	s := &http.Server{
		Handler:      mux,
		Addr:         addr,
		ReadTimeout:  viper.GetDuration("read_timeout"),
		WriteTimeout: viper.GetDuration("write_timeout"),
	}

	log.Printf("API server listening on http://%s", addr)
	log.Printf("Swagger UI available at http://%s/docs", addr)
	return s.ListenAndServe()
}

func main() {

	log.SetOutput(os.Stdout)

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

	// startCmd represents the 'start' subcommand
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

	// General Config flags
	startCmd.Flags().String("config", "", "Path to Viper config")
	startCmd.Flags().String("log-level", "info", "Log verbosity: debug, info")

	// API Server flags
	startCmd.Flags().IntP("port", "p", 8080, "Port to run the server on (e.g., 8080)")
	startCmd.Flags().String("host", "0.0.0.0", "Host address to bind")
	startCmd.Flags().Duration("read-timeout", 5*time.Second, "Max duration for reading the entire request (e.g. 5s)")
	startCmd.Flags().Duration("write-timeout", 10*time.Second, "Max duration before timing out writes")
	startCmd.Flags().Duration("graceful-timeout", 15*time.Second, "Time allowed for graceful shutdown")
	startCmd.Flags().String("database-engine", "etcd", "Specifies the backend database engine for persisting probe configurations (default: etcd)")
	startCmd.Flags().String("kubeconfig", "", "Path to kubeconfig file (optional, for out-of-cluster development)")
	startCmd.Flags().String("namespace", "default", "The Kubernetes namespace to store probe configmaps in.")

	// Bind flags to viper
	viper.BindPFlag("port", startCmd.Flags().Lookup("port"))
	viper.BindPFlag("host", startCmd.Flags().Lookup("host"))
	viper.BindPFlag("read_timeout", startCmd.Flags().Lookup("read-timeout"))
	viper.BindPFlag("write_timeout", startCmd.Flags().Lookup("write-timeout"))
	viper.BindPFlag("graceful_timeout", startCmd.Flags().Lookup("graceful-timeout"))
	viper.BindPFlag("database_engine", startCmd.Flags().Lookup("database-engine"))
	viper.BindPFlag("config", startCmd.Flags().Lookup("config"))
	viper.BindPFlag("log_level", startCmd.Flags().Lookup("log-level"))
	viper.BindPFlag("kubeconfig", startCmd.Flags().Lookup("kubeconfig"))
	viper.BindPFlag("namespace", startCmd.Flags().Lookup("namespace"))

	// Add commands to the root command
	rootCmd.AddCommand(startCmd)

	// Execute the root command. This parses the arguments and calls the appropriate command's Run function.
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
