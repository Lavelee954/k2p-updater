package app

import (
	"errors"
	"fmt"
	"github.com/spf13/viper"
)

// Config holds the complete application configuration
type Config struct {

	// Kubernetes configuration
	Kubernetes KubernetesConfig `mapstructure:"kubernetes"`

	// Resources configuration
	Resources ResourcesConfig `mapstructure:"resources"`
}

// KubernetesConfig holds Kubernetes client configuration
type KubernetesConfig struct {
	// Namespace is the default namespace for resources
	Namespace string `mapstructure:"namespace"`

	// ConfigPath is the path to the kubeconfig file
	ConfigPath string `mapstructure:"config_path"`

	// MasterURL is the Kubernetes API server URL
	MasterURL string `mapstructure:"master_url"`
}

// ResourcesConfig holds configuration for resources
type ResourcesConfig struct {
	// Namespace is the default namespace for resources
	Namespace string `mapstructure:"namespace"`

	// Group is the API group for resources
	Group string `mapstructure:"group"`

	// Version is the API version for resources
	Version string `mapstructure:"version"`

	// Definitions defines the resource types
	Definitions map[string]ResourceDefinitionConfig `mapstructure:"definitions"`
}

// ResourceDefinitionConfig holds the configuration for a resource definition
type ResourceDefinitionConfig struct {
	// Resource is the resource name (e.g., "k2pupdaters")
	Resource string `mapstructure:"resource"`

	// NameFormat is the name format (e.g., "k2pupdater-%s")
	NameFormat string `mapstructure:"name_format"`

	// StatusField can be either a string (e.g., "updateStatus") or a map of field definitions
	StatusField interface{} `mapstructure:"status_field"`

	// Kind is the optional resource kind
	Kind string `mapstructure:"kind"`
}

// Load loads configuration from files and environment
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Configure paths and file types
	configureViper(v)

	// Read configs file
	if err := readConfigs(v); err != nil {
		return nil, err
	}

	// Load environment variables from app.env
	if err := loadEnvVars(v); err != nil {
		return nil, err
	}

	// Unmarshal configuration
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configs: %w", err)
	}

	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// configureViper sets up Viper configuration paths and types
func configureViper(v *viper.Viper) {
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./configs")
	v.AddConfigPath("./properties")
	v.AddConfigPath("/etc/ks-updater/")

	// Enable environment variables
	v.AutomaticEnv()
	v.SetEnvPrefix("KS_UPDATER")
}

// readConfigs attempts to read the configuration file
func readConfigs(v *viper.Viper) error {
	if err := v.ReadInConfig(); err != nil {
		// Only return error if it's not a "configs file not found" error
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return fmt.Errorf("failed to read configs file: %w", err)
		}
		// Otherwise, continue with defaults and environment variables
	}
	return nil
}

// loadEnvVars loads environment variables from app.env file
func loadEnvVars(v *viper.Viper) error {
	envViper := viper.New()
	envViper.SetConfigName("app")
	envViper.SetConfigType("env")
	envViper.AddConfigPath("./configs")
	envViper.AddConfigPath("./properties")

	if err := envViper.ReadInConfig(); err == nil {
		// Merge environment file into main configs if found
		for _, key := range envViper.AllKeys() {
			v.Set(key, envViper.Get(key))
		}
	}
	return nil
}

// validateConfig validates the configuration
func validateConfig(cfg *Config) error {

	// Validate Kubernetes configuration
	if cfg.Kubernetes.Namespace == "" {
		return fmt.Errorf("kubernetes.namespace is required")
	}

	return nil
}

// setDefaults sets default values for configuration
func setDefaults(v *viper.Viper) {
	// Kubernetes defaults
	v.SetDefault("kubernetes.namespace", "default")

	v.SetDefault("resources.namespace", "default")
	v.SetDefault("resources.group", "k2p.cloud.kt.com")
	v.SetDefault("resources.version", "v1beta1")

	// Updater resource defaults
	v.SetDefault("resources.definitions.updater.resource", "k2pupdaters")
	v.SetDefault("resources.definitions.updater.name_format", "k2pupdater-%s")
	v.SetDefault("resources.definitions.updater.status_field", "updates")
	v.SetDefault("resources.definitions.updater.kind", "K2pUpdater")

	// Upgrade resource defaults
	v.SetDefault("resources.definitions.upgrader.resource", "k2pupgraders")
	v.SetDefault("resources.definitions.upgrader.name_format", "k2pupgrader-%s")
	v.SetDefault("resources.definitions.upgrader.status_field", "upgradeStatus")
	v.SetDefault("resources.definitions.upgrader.kind", "K2pUpgrader")
}

// GetStatusFieldString returns the status field as a string if it is a string
func (rdc *ResourceDefinitionConfig) GetStatusFieldString() (string, bool) {
	if field, ok := rdc.StatusField.(string); ok {
		return field, true
	}
	return "", false
}
