package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
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

func createKubernetesClientset() (*kubernetes.Clientset, error) {
	// Try to create in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		// If in-cluster fails, try to use kubeconfig
		log.Printf("Could not create in-cluster config: %v. Trying to use kubeconfig.", err)
		kubeconfigPath := viper.GetString("kubeconfig")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client config from kubeconfig: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}
	return clientset, nil
}

func createRouter(validatedAPI http.Handler, clientset *kubernetes.Clientset, swagger *openapi3.T) http.Handler {
	// The main router
	mux := http.NewServeMux()

	// Liveness and Readiness probes
	mux.HandleFunc("/livez", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// If not using the etcd backend, we don't need to check k8s connectivity.
		if clientset == nil {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		_, err := clientset.Discovery().ServerVersion()
		if err != nil {
			log.Printf("Readiness check failed: could not connect to Kubernetes API server: %v", err)
			http.Error(w, "not ready: failed to connect to Kubernetes", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

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

	// Mount the validated API router to the main router.
	// Requests will be matched against the UI handlers first, then fall through to the API.
	mux.Handle("/", validatedAPI)
	return mux
}

func createProbeStore() (probestore.ProbeStorage, *kubernetes.Clientset, error) {
	var store probestore.ProbeStorage
	var clientset *kubernetes.Clientset
	var err error

	databaseEngine := viper.GetString("database_engine")
	log.Printf("Using database engine: %s", databaseEngine)

	switch databaseEngine {
	case "etcd":
		clientset, err = createKubernetesClientset()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
		}
		namespace := viper.GetString("namespace")
		store, err = probestore.NewKubernetesProbeStore(context.Background(), clientset, namespace)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create kubernetes probe store: %w", err)
		}
	case "local":
		store, err = probestore.NewLocalProbeStore()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create local probe store: %w", err)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported database engine: %s. Supported engines are 'etcd', 'local'", databaseEngine)
	}
	return store, clientset, nil
}

// runWebServer starts the HTTP server.
func runWebServer(addr string) error {

	swagger, err := v1.GetSwagger()
	if err != nil {
		return fmt.Errorf("error loading swagger spec: %w", err)
	}

	swagger.Servers = nil

	store, clientset, err := createProbeStore()
	if err != nil {
		return fmt.Errorf("failed to create probe store: %w", err)
	}

	server := api.NewServer(store)
	serverHandler := v1.NewStrictHandler(server, nil)

	// The API handlers are registered on a separate router and validated.
	apiRouter := http.NewServeMux()
	v1.HandlerFromMux(serverHandler, apiRouter)
	validatedAPI := middleware.OapiRequestValidator(swagger)(apiRouter)

	router := createRouter(validatedAPI, clientset, swagger)

	s := &http.Server{
		Handler:      router,
		Addr:         addr,
		ReadTimeout:  viper.GetDuration("read_timeout"),
		WriteTimeout: viper.GetDuration("write_timeout"),
	}

	// Start the server in a goroutine so it doesn't block the main thread
	go func() {
		log.Printf("API server listening on http://%s", addr)
		log.Printf("Swagger UI available at http://%s/docs", addr)
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
		log.Println("Server stopped serving new connections.")
	}()

	// Set up a channel to listen for OS signals for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM) // Listen for Ctrl+C and termination signals

	// Block until a signal is received
	sig := <-quit
	log.Printf("Received signal: %v. Initiating graceful shutdown...", sig)

	// Create a deadline context for the shutdown process
	shutdownTimeout := viper.GetDuration("graceful_timeout")
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Attempt graceful shutdown
	if err := s.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server gracefully shut down.")

	return nil
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
	startCmd.Flags().String("database-engine", "etcd", "Specifies the backend database engine. Supported: 'etcd', 'local'.")
	startCmd.Flags().String("kubeconfig", "", "Path to kubeconfig file (optional, for out-of-cluster development)")
	startCmd.Flags().String("namespace", "default", "The Kubernetes namespace to store probe configmaps in.")

	// Bind flags to viper
	viper.BindPFlag("port", startCmd.Flags().Lookup("port")) //nolint:errcheck
	viper.BindPFlag("host", startCmd.Flags().Lookup("host")) //nolint:errcheck
	viper.BindPFlag("read_timeout", startCmd.Flags().Lookup("read-timeout")) //nolint:errcheck
	viper.BindPFlag("write_timeout", startCmd.Flags().Lookup("write-timeout")) //nolint:errcheck
	viper.BindPFlag("graceful_timeout", startCmd.Flags().Lookup("graceful-timeout")) //nolint:errcheck
	viper.BindPFlag("database_engine", startCmd.Flags().Lookup("database-engine")) //nolint:errcheck
	viper.BindPFlag("config", startCmd.Flags().Lookup("config")) //nolint:errcheck
	viper.BindPFlag("log_level", startCmd.Flags().Lookup("log-level")) //nolint:errcheck
	viper.BindPFlag("kubeconfig", startCmd.Flags().Lookup("kubeconfig")) //nolint:errcheck
	viper.BindPFlag("namespace", startCmd.Flags().Lookup("namespace")) //nolint:errcheck

	// Add commands to the root command
	rootCmd.AddCommand(startCmd)

	// Execute the root command. This parses the arguments and calls the appropriate command's Run function.
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
