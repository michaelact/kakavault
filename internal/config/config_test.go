package config

import (
	"os"
	"path/filepath"
	"testing"
)

const validYAML = `
namespace_pattern: "^(?P<repo>.+)-(?P<environment>[a-z0-9]+)$"
secret_name_pattern: "^(?P<application>.+)-secret-variables$"
vault:
  mount: default
  kv_version: 2
classification:
  rules:
    - pattern: "_API_KEY$"
      classification: third-party
    - pattern: ".*"
      classification: internal
  overrides:
    ADMIN_API_KEY: internal
`

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoad_Valid(t *testing.T) {
	path := writeTempConfig(t, validYAML)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.NamespacePattern != "^(?P<repo>.+)-(?P<environment>[a-z0-9]+)$" {
		t.Errorf("NamespacePattern = %q, want the pattern from YAML", cfg.NamespacePattern)
	}
	if cfg.Vault.Mount != "default" {
		t.Errorf("Vault.Mount = %q, want %q", cfg.Vault.Mount, "default")
	}
	if cfg.Vault.KVVersion != 2 {
		t.Errorf("Vault.KVVersion = %d, want 2", cfg.Vault.KVVersion)
	}
	if len(cfg.Classification.Rules) != 2 {
		t.Fatalf("len(Classification.Rules) = %d, want 2", len(cfg.Classification.Rules))
	}
	if cfg.Classification.Rules[0].Pattern != "_API_KEY$" {
		t.Errorf("Rules[0].Pattern = %q, want %q", cfg.Classification.Rules[0].Pattern, "_API_KEY$")
	}
	if got := cfg.Classification.Overrides["ADMIN_API_KEY"]; got != "internal" {
		t.Errorf("Overrides[ADMIN_API_KEY] = %q, want %q", got, "internal")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("Load() error = nil, want an error for a missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTempConfig(t, "not: [valid: yaml")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want an error for invalid YAML")
	}
}

func TestValidate_MissingNamespacePattern(t *testing.T) {
	cfg := &Config{
		SecretNamePattern: "^(?P<application>.+)-secret-variables$",
		Vault:             VaultConfig{Mount: "default", KVVersion: 2},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want an error for missing NamespacePattern")
	}
}

func TestValidate_BadRegex(t *testing.T) {
	cfg := &Config{
		NamespacePattern:  "(unclosed",
		SecretNamePattern: "^(?P<application>.+)-secret-variables$",
		Vault:             VaultConfig{Mount: "default", KVVersion: 2},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want an error for an invalid regex")
	}
}

func TestValidate_BadKVVersion(t *testing.T) {
	cfg := &Config{
		NamespacePattern:  "^(?P<repo>.+)-(?P<environment>[a-z0-9]+)$",
		SecretNamePattern: "^(?P<application>.+)-secret-variables$",
		Vault:             VaultConfig{Mount: "default", KVVersion: 3},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want an error for kv_version 3")
	}
}

func TestValidate_OK(t *testing.T) {
	cfg := &Config{
		NamespacePattern:  "^(?P<repo>.+)-(?P<environment>[a-z0-9]+)$",
		SecretNamePattern: "^(?P<application>.+)-secret-variables$",
		Vault:             VaultConfig{Mount: "default", KVVersion: 2},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}
