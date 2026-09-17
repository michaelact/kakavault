package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// ClassificationRule matches a Secret key name against Pattern (a regex) and,
// on match, assigns it Classification.
type ClassificationRule struct {
	Pattern        string `yaml:"pattern"`
	Classification string `yaml:"classification"`
}

// Classification holds the ordered rule list (first match wins) plus an
// overrides map that always wins over the rules, keyed by exact key name.
type Classification struct {
	Rules     []ClassificationRule `yaml:"rules"`
	Overrides map[string]string    `yaml:"overrides"`
}

// VaultConfig configures which Vault KV mount to write into.
type VaultConfig struct {
	Mount     string `yaml:"mount"`
	KVVersion int    `yaml:"kv_version"`
}

// Config is the full k2v configuration, loaded from a single YAML file.
type Config struct {
	NamespacePattern  string          `yaml:"namespace_pattern"`
	SecretNamePattern string          `yaml:"secret_name_pattern"`
	Vault             VaultConfig     `yaml:"vault"`
	Classification    Classification  `yaml:"classification"`
}

// Load reads and parses the YAML config file at path. It does not validate
// the result — call Validate() separately.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	return &cfg, nil
}

// Validate checks that required fields are present and that regex patterns
// and the KV version are actually usable.
func (c *Config) Validate() error {
	if c.NamespacePattern == "" {
		return fmt.Errorf("namespace_pattern is required")
	}
	if c.SecretNamePattern == "" {
		return fmt.Errorf("secret_name_pattern is required")
	}
	if _, err := regexp.Compile(c.NamespacePattern); err != nil {
		return fmt.Errorf("namespace_pattern is not a valid regex: %w", err)
	}
	if _, err := regexp.Compile(c.SecretNamePattern); err != nil {
		return fmt.Errorf("secret_name_pattern is not a valid regex: %w", err)
	}
	if c.Vault.Mount == "" {
		return fmt.Errorf("vault.mount is required")
	}
	if c.Vault.KVVersion != 1 && c.Vault.KVVersion != 2 {
		return fmt.Errorf("vault.kv_version must be 1 or 2, got %d", c.Vault.KVVersion)
	}
	for i, rule := range c.Classification.Rules {
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return fmt.Errorf("classification.rules[%d].pattern is not a valid regex: %w", i, err)
		}
		if rule.Classification == "" {
			return fmt.Errorf("classification.rules[%d].classification is required", i)
		}
	}
	return nil
}
