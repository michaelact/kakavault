// Package discover finds every (namespace, Secret) pair in the cluster
// whose namespace matches namespace_pattern and whose Secret name matches
// secret_name_pattern — the --all mode's cluster-wide equivalent of
// manually naming one --namespace/--secret pair.
package discover

import (
	"context"
	"fmt"
	"regexp"
)

// systemNamespaces are never scanned — they'd never match a
// repo-environment style namespace_pattern anyway, so skipping them
// avoids pointless List calls (and needing list-secrets RBAC there).
var systemNamespaces = map[string]bool{
	"kube-system":     true,
	"kube-public":     true,
	"kube-node-lease": true,
}

// Target is one matched (namespace, Secret name) pair.
type Target struct {
	Namespace  string
	SecretName string
}

// Lister is the subset of k8sreader.Reader that Find needs.
type Lister interface {
	ListNamespaces(ctx context.Context) ([]string, error)
	ListSecrets(ctx context.Context, namespace string) ([]string, error)
}

// Find lists every namespace, filters to ones matching namespacePattern,
// then lists each matching namespace's Secrets and filters those to ones
// matching secretNamePattern. Non-matches are skipped, not errors — most
// namespaces/Secrets in a cluster aren't the ones being migrated.
func Find(ctx context.Context, lister Lister, namespacePattern, secretNamePattern string) ([]Target, error) {
	nsRe, err := regexp.Compile(namespacePattern)
	if err != nil {
		return nil, fmt.Errorf("compile namespace_pattern: %w", err)
	}

	namespaces, err := lister.ListNamespaces(ctx)
	if err != nil {
		return nil, err
	}

	var targets []Target
	for _, ns := range namespaces {
		if systemNamespaces[ns] || !nsRe.MatchString(ns) {
			continue
		}

		t, err := FindInNamespace(ctx, lister, ns, secretNamePattern)
		if err != nil {
			return nil, err
		}
		targets = append(targets, t...)
	}

	return targets, nil
}

// FindInNamespace lists namespace's Secrets and returns the ones whose
// name matches secretNamePattern. Unlike Find, the namespace itself is
// taken as given — no namespace_pattern check, no system-namespace skip
// (the caller already decided this specific namespace is in scope).
func FindInNamespace(ctx context.Context, lister Lister, namespace, secretNamePattern string) ([]Target, error) {
	secretRe, err := regexp.Compile(secretNamePattern)
	if err != nil {
		return nil, fmt.Errorf("compile secret_name_pattern: %w", err)
	}

	secrets, err := lister.ListSecrets(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("list secrets in %s: %w", namespace, err)
	}

	var targets []Target
	for _, secretName := range secrets {
		if secretRe.MatchString(secretName) {
			targets = append(targets, Target{Namespace: namespace, SecretName: secretName})
		}
	}

	return targets, nil
}
