// Package pathresolver parses a namespace name and Secret name into their
// repo/environment/application components, and builds the Vault KV path
// for a given key once classified.
package pathresolver

import (
	"fmt"
	"regexp"
)

// Resolved holds the path components extracted from a namespace and
// Secret name.
type Resolved struct {
	Repo        string
	Environment string
	Application string
}

// Resolve runs namespacePattern against namespace and secretNamePattern
// against secretName, extracting the "repo"/"environment" and
// "application" named capture groups respectively. Both patterns must
// define those exact group names.
func Resolve(namespacePattern, secretNamePattern, namespace, secretName string) (Resolved, error) {
	nsRe, err := regexp.Compile(namespacePattern)
	if err != nil {
		return Resolved{}, fmt.Errorf("compile namespace_pattern: %w", err)
	}
	secretRe, err := regexp.Compile(secretNamePattern)
	if err != nil {
		return Resolved{}, fmt.Errorf("compile secret_name_pattern: %w", err)
	}

	repo, environment, ok := extractTwo(nsRe, namespace, "repo", "environment")
	if !ok {
		return Resolved{}, fmt.Errorf("namespace %q does not match namespace_pattern %q", namespace, namespacePattern)
	}

	application, ok := extractOne(secretRe, secretName, "application")
	if !ok {
		return Resolved{}, fmt.Errorf("secret name %q does not match secret_name_pattern %q", secretName, secretNamePattern)
	}

	return Resolved{Repo: repo, Environment: environment, Application: application}, nil
}

func extractOne(re *regexp.Regexp, input, group string) (string, bool) {
	match := re.FindStringSubmatch(input)
	if match == nil {
		return "", false
	}
	idx := re.SubexpIndex(group)
	if idx < 0 || idx >= len(match) {
		return "", false
	}
	return match[idx], true
}

func extractTwo(re *regexp.Regexp, input, group1, group2 string) (string, string, bool) {
	match := re.FindStringSubmatch(input)
	if match == nil {
		return "", "", false
	}
	idx1, idx2 := re.SubexpIndex(group1), re.SubexpIndex(group2)
	if idx1 < 0 || idx1 >= len(match) || idx2 < 0 || idx2 >= len(match) {
		return "", "", false
	}
	return match[idx1], match[idx2], true
}

// BuildPath returns the Vault KV subpath for one key, WITHOUT a mount
// prefix — the caller (vaultwriter) owns and prepends the mount.
func BuildPath(repo, environment, application, classification, key string) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s", repo, environment, application, classification, key)
}
