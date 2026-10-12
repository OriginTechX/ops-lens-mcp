package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the top-level configuration for ops-lens-mcp.
type Config struct {
	Defaults Defaults          `yaml:"defaults"`
	Targets  map[string]Target `yaml:"targets"`
}

// Defaults defines global limits and behavior.
type Defaults struct {
	MaxLogLines int  `yaml:"max_log_lines"`
	MaxBytes    int  `yaml:"max_bytes"`
	StripANSI   bool `yaml:"strip_ansi"`
}

// Target defines an infrastructure environment (K8s or Docker Compose via SSH).
type Target struct {
	Driver string `yaml:"driver"` // "k8s" or "docker-compose"

	// K8s specific
	Kubeconfig       string   `yaml:"kubeconfig,omitempty"`
	Context          string   `yaml:"context,omitempty"`
	Namespace        string   `yaml:"namespace,omitempty"`
	AllowedNamespaces []string `yaml:"allow_namespaces,omitempty"`

	// Docker-compose specific
	SSH         *SSHConfig        `yaml:"ssh,omitempty"`
	ComposePath string            `yaml:"compose_path,omitempty"`
	Services    map[string]string `yaml:"services,omitempty"` // service name -> container name
}

// SSHConfig holds connection details for remote VPS inspection.
type SSHConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	KeyFile  string `yaml:"key_file"`
	Password string `yaml:"password,omitempty"`
}

var envVarPattern = regexp.MustCompile(`\$\{([a-zA-Z_0-9]+)(?::-([^}]*))?\}`)

// ExpandEnv replaces ${VAR} or ${VAR:-default} with environment variables.
func ExpandEnv(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		submatches := envVarPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		varName := submatches[1]
		defaultVal := ""
		if len(submatches) > 2 {
			defaultVal = submatches[2]
		}
		if val, ok := os.LookupEnv(varName); ok && val != "" {
			return val
		}
		return defaultVal
	})
}

// Load reads and parses a YAML configuration file with env expansion.
func Load(path string) (*Config, error) {
	// Expand tilde if path starts with ~/
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	expanded := ExpandEnv(string(raw))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}

	// Apply safe defaults
	if cfg.Defaults.MaxLogLines <= 0 {
		cfg.Defaults.MaxLogLines = 250
	}
	if cfg.Defaults.MaxBytes <= 0 {
		cfg.Defaults.MaxBytes = 64 * 1024 // 64 KB
	}

	return &cfg, nil
}
