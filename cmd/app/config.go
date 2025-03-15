package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds the complete application configuration
type Config struct {
	// Server configuration
	Server ServerConfig `mapstructure:"server"`

	// Kubernetes configuration
	Kubernetes KubernetesConfig `mapstructure:"kubernetes"`

	// Metrics configuration
	Metrics MetricsConfig `mapstructure:"metrics"`

	// Updater configuration
	Updater UpdaterConfig `mapstructure:"updater"`

	// Exporter configuration
	Exporter ExporterConfig `mapstructure:"exporter"`

	// Backend configuration
	Backend BackendConfig `mapstructure:"backend"`

	// Application configuration
	App AppConfig `mapstructure:"app"`

	// Resources configuration
	Resources ResourcesConfig `mapstructure:"resources"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	// Port is the HTTP server port
	Port string `mapstructure:"port"`

	// ShutdownTimeout is the timeout for server shutdown
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
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

// MetricsConfig holds metrics collection configuration
type MetricsConfig struct {
	// WindowSize is the size of the monitoring window
	WindowSize time.Duration `mapstructure:"window_size"`

	// SlidingSize is the interval between metrics collection
	SlidingSize time.Duration `mapstructure:"sliding_size"`

	// CooldownPeriod is the cooldown period after scaling
	CooldownPeriod time.Duration `mapstructure:"cooldown_period"`

	// ScaleTrigger is the CPU threshold for scaling
	ScaleTrigger float64 `mapstructure:"scale_trigger"`
}

// UpdaterConfig holds updater service configuration
type UpdaterConfig struct {
	// ScaleThreshold is the CPU threshold for scaling
	ScaleThreshold float64 `mapstructure:"scale_threshold"`

	// ScaleUpStep is the amount to scale up
	ScaleUpStep int32 `mapstructure:"scale_up_step"`

	// CooldownPeriod is the cooldown period after scaling
	CooldownPeriod time.Duration `mapstructure:"cooldown_period"`
}

// ExporterConfig holds exporter service configuration
type ExporterConfig struct {
	// Namespace is the namespace for node exporters
	Namespace string `mapstructure:"namespace"`

	// AppLabel is the label selector for node exporters
	AppLabel string `mapstructure:"app_label"`

	// UpdateInterval is the interval for updating exporters
	UpdateInterval time.Duration `mapstructure:"update_interval"`

	// RetryInterval is the interval for retrying health checks
	RetryInterval time.Duration `mapstructure:"retry_interval"`

	// MaxRetries is the maximum number of health check retries
	MaxRetries int `mapstructure:"max_retries"`

	// HealthCheckTimeout is the timeout for health checks
	HealthCheckTimeout time.Duration `mapstructure:"health_check_timeout"`

	// MetricsPort is the port for metrics endpoints
	MetricsPort int `mapstructure:"metrics_port"`
}

// BackendConfig holds backend API configuration
type BackendConfig struct {
	// BaseURL is the base URL for the backend API
	BaseURL string `mapstructure:"base_url"`

	// APIKey is the API key for the backend API
	APIKey string `mapstructure:"api_key"`

	// Timeout is the timeout for backend API requests
	Timeout time.Duration `mapstructure:"timeout"`

	// SecretName is the name of the Kubernetes secret containing credentials
	SecretName string `mapstructure:"secret_name"`

	// AuthPath is the API path for authentication
	AuthPath string `mapstructure:"auth_path"`

	// BaseURLKey is the key in the secret for the base URL
	BaseURLKey string `mapstructure:"base_url_key"`

	// SourceComponent is the component identifier for authentication
	SourceComponent string `mapstructure:"source_component"`
}

// AppConfig holds application configuration
type AppConfig struct {
	// Component is the name of the component
	Component string `mapstructure:"component"`

	// LogLevel is the log level
	LogLevel string `mapstructure:"log_level"`
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

	// CRName is the name to use in the format string (fixed value or "%s" for node name)
	CRName string `mapstructure:"cr_name"`
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
	// Validate server configuration
	if cfg.Server.Port == "" {
		return fmt.Errorf("server.port is required")
	}

	// Validate Kubernetes configuration
	if cfg.Kubernetes.Namespace == "" {
		return fmt.Errorf("kubernetes.namespace is required")
	}

	// Validate metrics configuration
	if cfg.Metrics.WindowSize <= 0 {
		return fmt.Errorf("metrics.window_size must be positive")
	}
	if cfg.Metrics.SlidingSize <= 0 {
		return fmt.Errorf("metrics.sliding_size must be positive")
	}
	if cfg.Metrics.ScaleTrigger <= 0 {
		return fmt.Errorf("metrics.scale_trigger must be positive")
	}

	// Validate updater configuration
	if cfg.Updater.ScaleThreshold <= 0 {
		return fmt.Errorf("updater.scale_threshold must be positive")
	}

	// Validate backend configuration
	if cfg.Backend.BaseURL == "" {
		return fmt.Errorf("backend.base_url is required")
	}
	if cfg.Backend.BaseURL == "" {
		return fmt.Errorf("backend.base_url is required")
	}
	if cfg.Backend.SecretName != "" && cfg.Backend.AuthPath == "" {
		return fmt.Errorf("backend.auth_path is required when secret_name is provided")
	}

	return nil
}

// setDefaults sets default values for configuration
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.port", ":8080")
	v.SetDefault("server.shutdown_timeout", 10*time.Second)

	// Kubernetes defaults
	v.SetDefault("kubernetes.namespace", "default")

	// Metrics defaults
	v.SetDefault("metrics.window_size", 10*time.Minute)
	v.SetDefault("metrics.sliding_size", 1*time.Minute)
	v.SetDefault("metrics.cooldown_period", 5*time.Minute)
	v.SetDefault("metrics.scale_trigger", 50.0)

	// Updater defaults
	v.SetDefault("updater.scale_threshold", 50.0)
	v.SetDefault("updater.scale_up_step", 1)
	v.SetDefault("updater.cooldown_period", 5*time.Minute)

	// Exporter defaults
	v.SetDefault("exporter.namespace", "monitoring")
	v.SetDefault("exporter.app_label", "app=node-exporter")
	v.SetDefault("exporter.update_interval", 30*time.Second)
	v.SetDefault("exporter.retry_interval", 5*time.Second)
	v.SetDefault("exporter.max_retries", 3)
	v.SetDefault("exporter.health_check_timeout", 5*time.Second)
	v.SetDefault("exporter.metrics_port", 9100)

	// Backend defaults
	v.SetDefault("backend.base_url", "https://api.backend-service.com/v1")
	v.SetDefault("backend.timeout", 30*time.Second)
	v.SetDefault("backend.secret_name", "backend-credentials")
	v.SetDefault("backend.auth_path", "auth/token")
	v.SetDefault("backend.base_url_key", "baseUrl")
	v.SetDefault("backend.source_component", "k2p-updater")

	// App defaults
	v.SetDefault("app.component", "k2p-updater")
	v.SetDefault("app.log_level", "info")

	v.SetDefault("resources.namespace", "default")
	v.SetDefault("resources.group", "k2p.cloud.kt.com")
	v.SetDefault("resources.version", "v1beta1")

	// Resource defaults
	v.SetDefault("resources.namespace", "default")
	v.SetDefault("resources.group", "k2p.cloud.kt.com")
	v.SetDefault("resources.version", "v1beta1")

	// Updater resource defaults
	v.SetDefault("resources.definitions.updater.resource", "k2pupdaters")
	v.SetDefault("resources.definitions.updater.kind", "K2pUpdater")
	v.SetDefault("resources.definitions.updater.name_format", "k2pupdater-%s")
	v.SetDefault("resources.definitions.updater.cr_name", "master")
	v.SetDefault("resources.definitions.updater.status_field", "updates")

	// Upgrade resource defaults
	v.SetDefault("resources.definitions.upgrader.resource", "k2pupgraders")
	v.SetDefault("resources.definitions.upgrader.name_format", "k2pupgrader-%s")
	v.SetDefault("resources.definitions.upgrader.status_field", "upgradeStatus")
	v.SetDefault("resources.definitions.upgrader.cr_name", "master")
	v.SetDefault("resources.definitions.upgrader.kind", "K2pUpgrader")
}

// GetStatusFieldString returns the status field as a string if it is a string
func (rdc *ResourceDefinitionConfig) GetStatusFieldString() (string, bool) {
	if field, ok := rdc.StatusField.(string); ok {
		return field, true
	}
	return "", false
}
