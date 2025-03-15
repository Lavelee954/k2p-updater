package app

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// KubeClientInterface is an interface that defines only the necessary methods of a Kubernetes clientset.
// This interface implements both real clientsets and fake clientsets.
type KubeClientInterface interface {
	CoreV1() typedcorev1.CoreV1Interface
}

// KubeClients holds the Kubernetes client instances.
type KubeClients struct {
	// ClientSet is the Kubernetes clientset that implements KubeClientInterface
	ClientSet KubeClientInterface

	// FullClientSet is the complete Kubernetes clientset
	FullClientSet kubernetes.Interface

	// DynamicClient is the Kubernetes dynamic client
	DynamicClient dynamic.Interface

	// Config is the Kubernetes REST configs
	Config *rest.Config
}

// NewKubeClients returns configured Kubernetes clients.
// It first tries to use a kubeconfig file, then falls back to in-cluster configuration.
func NewKubeClients(cfg *KubernetesConfig) (*KubeClients, error) {
	config, err := getKubeConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Create dynamic client
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &KubeClients{
		ClientSet:     clientset,
		FullClientSet: clientset, // Store the full clientset as well
		DynamicClient: dynamicClient,
		Config:        config,
	}, nil
}

// getKubeConfig returns the kubernetes REST configuration
func getKubeConfig(cfg *KubernetesConfig) (*rest.Config, error) {
	// Determine kubeconfig file location
	kubeconfig := determineKubeconfigPath(cfg.ConfigPath)

	// Check if we should use in-cluster configs
	useInCluster := shouldUseInClusterConfig(kubeconfig)

	if useInCluster {
		// Use in-cluster configs
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create in-cluster configs: %w", err)
		}
		return config, nil
	}

	// Use the kubeconfig file
	config, err := clientcmd.BuildConfigFromFlags(cfg.MasterURL, kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build configs from kubeconfig %s: %w", kubeconfig, err)
	}
	return config, nil
}

// determineKubeconfigPath finds the kubeconfig file path
func determineKubeconfigPath(configPath string) string {
	if configPath != "" {
		return configPath
	}

	if path := os.Getenv("KUBECONFIG"); path != "" {
		return path
	}

	if home := homedir.HomeDir(); home != "" {
		return filepath.Join(home, ".kube", "config")
	}

	return ""
}

// shouldUseInClusterConfig determines if in-cluster config should be used
func shouldUseInClusterConfig(kubeconfig string) bool {
	if kubeconfig == "" {
		return true
	}

	_, err := os.Stat(kubeconfig)
	return err != nil
}
