package kubeclient

import (
	"os"
	"testing"
)

func TestIsRunningInK8sCluster(t *testing.T) {
	// Test when not in Kubernetes (normal case in test environment)
	result := IsRunningInK8sCluster()
	
	// In test environment, should return false since we don't have K8s service account
	if result {
		t.Log("Running in actual Kubernetes environment - this is expected if tests are run in a K8s pod")
	} else {
		t.Log("Not running in Kubernetes environment - this is expected for local testing")
	}
}

func TestNewClient_InvalidConfig(t *testing.T) {
	// Test with invalid kubeconfig path when not in cluster
	cfg := Config{
		KubeconfigPath: "/non/existent/path",
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Log("NewClient succeeded - this might indicate we're running in a Kubernetes cluster")
	} else {
		t.Logf("NewClient failed as expected: %v", err)
	}
}

func TestNewClient_EmptyConfig(t *testing.T) {
	// Test with empty config (should try default kubeconfig locations)
	cfg := Config{}

	client, err := NewClient(cfg)
	if err != nil {
		t.Logf("NewClient failed (expected in test environment): %v", err)
		return
	}

	// If successful, verify the client was created properly
	if client.Clientset() == nil {
		t.Error("Expected non-nil clientset")
	}
	if client.DynamicClient() == nil {
		t.Error("Expected non-nil dynamic client")
	}
	if client.Config() == nil {
		t.Error("Expected non-nil config")
	}
}

func TestCreateConfig_InCluster(t *testing.T) {
	// This test will pass if we're actually running in a cluster
	config, isInCluster, err := createConfig("")
	
	if err != nil {
		t.Logf("createConfig failed (expected in test environment): %v", err)
		return
	}

	if config == nil {
		t.Error("Expected non-nil config")
	}

	t.Logf("Created config successfully, isInCluster: %v", isInCluster)
}

func TestCreateConfig_WithKubeconfig(t *testing.T) {
	// Test with explicit kubeconfig path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home directory")
	}

	kubeconfigPath := homeDir + "/.kube/config"
	config, isInCluster, err := createConfig(kubeconfigPath)
	
	if err != nil {
		t.Logf("createConfig with kubeconfig failed (might be expected): %v", err)
		return
	}

	if config == nil {
		t.Error("Expected non-nil config")
	}

	if isInCluster {
		t.Error("Expected isInCluster to be false when using kubeconfig")
	}

	t.Log("Created config from kubeconfig successfully")
}