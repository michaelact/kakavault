package main

import (
	"context"
	"fmt"
	"io"

	"github.com/michaelact/k2v/internal/config"
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

	// Validate the namespace/secret-name patterns before touching the
	// cluster at all — a pattern mismatch is a config problem, not
	// something a k8s API failure should mask.
	if _, err := pathresolver.Resolve(cfg.NamespacePattern, cfg.SecretNamePattern, args.namespace, args.secretName); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	reader := k8sreader.New(k8sClient)
	secretData, err := reader.FetchSecret(context.Background(), args.namespace, args.secretName)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	items, err := migrate.Plan(cfg, secretData, args.namespace, args.secretName)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	if !args.apply {
		report.PrintPlanned(stdout, items)
		fmt.Fprintln(stdout, "\nDry run — no changes made. Re-run with --apply to write these to Vault.")
		return 0
	}

	results := migrate.Apply(context.Background(), vaultWriter, items, secretData)
	failed := report.PrintResults(stdout, results)
	if failed > 0 {
		fmt.Fprintf(stderr, "\n%d key(s) failed to migrate — see table above.\n", failed)
		return 1
	}
	return 0
}
