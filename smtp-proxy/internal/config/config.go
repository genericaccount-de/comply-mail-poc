// Package config loads and validates the SMTP proxy configuration
// from a YAML file (listen addr, upstream host/port, backend API URL,
// review mailbox, metrics address).
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level SMTP proxy configuration.
type Config struct {
	// ListenAddr is the address the proxy listens on, e.g. ":2525".
	ListenAddr string         `yaml:"listen_addr"`
	Upstream   UpstreamConfig `yaml:"upstream"`
	Backend    BackendConfig  `yaml:"backend"`
	Review     ReviewConfig   `yaml:"review"`
	// MetricsAddr is the address the metrics HTTP endpoint listens on,
	// e.g. ":2526".
	MetricsAddr string `yaml:"metrics_addr"`
}

// UpstreamConfig points at the customer's real mail server.
type UpstreamConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// BackendConfig points at the ComplyMail backend API.
type BackendConfig struct {
	// APIURL is the base URL used to reach the backend, e.g. "http://api:8080".
	APIURL string `yaml:"api_url"`
	// TimeoutSeconds bounds each scan request to the backend.
	TimeoutSeconds int `yaml:"timeout_seconds"`
}

// ReviewConfig identifies where flagged/redirected emails should be sent.
type ReviewConfig struct {
	// Mailbox is the review mailbox address. If empty, redirected emails
	// are still header-stamped but delivered to their original recipients.
	Mailbox string `yaml:"mailbox"`
}

// Default values applied when the corresponding field is empty.
const (
	DefaultListenAddr            = ":2525"
	DefaultUpstreamPort          = 25
	DefaultBackendTimeoutSeconds = 5
	DefaultMetricsAddr           = ":2526"
)

// Load reads, parses, and validates the YAML config at path, applying defaults.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %q: %w", path, err)
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.ListenAddr == "" {
		c.ListenAddr = DefaultListenAddr
	}
	if c.Upstream.Port == 0 {
		c.Upstream.Port = DefaultUpstreamPort
	}
	if c.Backend.TimeoutSeconds == 0 {
		c.Backend.TimeoutSeconds = DefaultBackendTimeoutSeconds
	}
	if c.MetricsAddr == "" {
		c.MetricsAddr = DefaultMetricsAddr
	}
}

func (c *Config) validate() error {
	if c.Upstream.Host == "" {
		return fmt.Errorf("config: upstream.host is required")
	}
	if c.Backend.APIURL == "" {
		return fmt.Errorf("config: backend.api_url is required")
	}
	return nil
}
