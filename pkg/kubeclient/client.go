package kubeclient

import (
	"fmt"
	"log"
	"os"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client provides a unified interface for Kubernetes client operations
type Client struct {
	config        *rest.Config
	clientset     kubernetes.Interface
	dynamicClient dynamic.Interface
	isInCluster   bool
}

// Config holds configuration options for creating a Kubernetes client
type Config struct {
	KubeconfigPath string
}

// NewClient creates a new Kubernetes client with the provided configuration
func NewClient(cfg Config) (*Client, error) {
	config, isInCluster, err := createConfig(cfg.KubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes dynamic client: %w", err)
	}

	return &Client{
		config:        config,
		clientset:     clientset,
		dynamicClient: dynamicClient,
		isInCluster:   isInCluster,
	}, nil
}

// Clientset returns the standard Kubernetes clientset
func (c *Client) Clientset() kubernetes.Interface {
	return c.clientset
}

// DynamicClient returns the dynamic Kubernetes client
func (c *Client) DynamicClient() dynamic.Interface {
	return c.dynamicClient
}

// Config returns the underlying rest.Config
func (c *Client) Config() *rest.Config {
	return c.config
}

// IsInCluster returns true if the client was created using in-cluster configuration
func (c *Client) IsInCluster() bool {
	return c.isInCluster
}

// IsRunningInK8sCluster checks if the current environment is a Kubernetes cluster
func IsRunningInK8sCluster() bool {
	// Check for service account token file (standard in K8s pods)
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		return true
	}

	// Check for KUBERNETES_SERVICE_HOST environment variable
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}

	return false
}

// createConfig creates a Kubernetes client configuration
func createConfig(kubeconfigPath string) (*rest.Config, bool, error) {
	// If explicit kubeconfig path is provided, use it directly
	if kubeconfigPath != "" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, false, fmt.Errorf("failed to create kubernetes client config from kubeconfig: %w", err)
		}
		return config, false, nil
	}

	// Try to create in-cluster config first
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, true, nil
	}

	// If in-cluster fails, try to use kubeconfig from default locations
	log.Printf("Could not create in-cluster config: %v. Trying to use kubeconfig.", err)
	config, err = clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		return nil, false, fmt.Errorf("failed to create kubernetes client config from kubeconfig: %w", err)
	}

	return config, false, nil
}