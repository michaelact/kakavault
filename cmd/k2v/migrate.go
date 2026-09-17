package main

import (
	"context"
	"fmt"
	"io"

	"github.com/michaelact/k2v/internal/config"
	"github.com/michaelact/k2v/internal/discover"
	"github.com/michaelact/k2v/internal/k8sreader"
	"github.com/michaelact/k2v/internal/migrate"
	"github.com/michaelact/k2v/internal/pathresolver"
	"github.com/michaelact/k2v/internal/report"
	"k8s.io/client-go/kubernetes"
)

// migrateArgs holds the parsed CLI flags for the migrate command.
type migrateArgs struct {
	namespace  string
	secretName string
	configPath string
	apply      bool
	all        bool // discover every matching (namespace, Secret) pair in the cluster instead of a single --namespace/--secret
}

// runMigrate is the testable core of the migrate command: it takes an
// already-constructed k8s client and (when applying) a VaultWriter, so
// tests never need a real cluster or Vault server. main() builds the
// real ones and calls this.
func runMigrate(args migrateArgs, k8sClient kubernetes.Interface, vaultWriter migrate.VaultWriter, stdout, stderr io.Writer) int {
	cfg, err := config.Load(args.configPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(stderr, "error: invalid config: %v\n", err)
		return 1
	}

	reader := k8sreader.New(k8sClient)

	if args.all {
		return runMigrateAll(context.Background(), cfg, reader, vaultWriter, args.apply, stdout, stderr)
	}

	// Validate the namespace/secret-name patterns before touching the
	// cluster at all — a pattern mismatch is a config problem, not
	// something a k8s API failure should mask.
	if _, err := pathresolver.Resolve(cfg.NamespacePattern, cfg.SecretNamePattern, args.namespace, args.secretName); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	failed, err := runOneTarget(context.Background(), cfg, reader, vaultWriter, args.namespace, args.secretName, args.apply, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if failed > 0 {
		fmt.Fprintf(stderr, "\n%d key(s) failed to migrate — see table above.\n", failed)
		return 1
	}
	return 0
}

// runMigrateAll discovers every (namespace, Secret) pair matching cfg's
// patterns, then runs each through the same logic as a single --namespace/
// --secret invocation, printing one header + table per target.
func runMigrateAll(ctx context.Context, cfg *config.Config, reader *k8sreader.Reader, vaultWriter migrate.VaultWriter, apply bool, stdout, stderr io.Writer) int {
	targets, err := discover.Find(ctx, reader, cfg.NamespacePattern, cfg.SecretNamePattern)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	if len(targets) == 0 {
		fmt.Fprintln(stdout, "No namespace/Secret pairs matched namespace_pattern and secret_name_pattern — nothing to do.")
		return 0
	}

	totalFailed := 0
	for _, target := range targets {
		fmt.Fprintf(stdout, "\n== %s / %s ==\n", target.Namespace, target.SecretName)
		failed, err := runOneTarget(ctx, cfg, reader, vaultWriter, target.Namespace, target.SecretName, apply, stdout)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s/%s: %v\n", target.Namespace, target.SecretName, err)
			totalFailed++
			continue
		}
		totalFailed += failed
	}

	if totalFailed > 0 {
		fmt.Fprintf(stderr, "\n%d key(s)/target(s) failed to migrate — see tables above.\n", totalFailed)
		return 1
	}
	return 0
}

// runOneTarget plans (and, if apply, applies) one namespace/Secret pair,
// printing its table to stdout. It returns the number of failed keys —
// callers decide how that maps to a process exit code.
func runOneTarget(ctx context.Context, cfg *config.Config, reader *k8sreader.Reader, vaultWriter migrate.VaultWriter, namespace, secretName string, apply bool, stdout io.Writer) (int, error) {
	secretData, err := reader.FetchSecret(ctx, namespace, secretName)
	if err != nil {
		return 0, err
	}

	items, err := migrate.Plan(cfg, secretData, namespace, secretName)
	if err != nil {
		return 0, err
	}

	if !apply {
		report.PrintPlanned(stdout, items)
		fmt.Fprintln(stdout, "Dry run — no changes made. Re-run with --apply to write these to Vault.")
		return 0, nil
	}

	results := migrate.Apply(ctx, vaultWriter, items, secretData)
	failed := report.PrintResults(stdout, results)
	return failed, nil
}
