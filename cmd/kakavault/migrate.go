package main

import (
	"context"
	"fmt"
	"io"
	"regexp"

	"github.com/michaelact/kakavault/internal/config"
	"github.com/michaelact/kakavault/internal/discover"
	"github.com/michaelact/kakavault/internal/k8sreader"
	"github.com/michaelact/kakavault/internal/migrate"
	"github.com/michaelact/kakavault/internal/pathresolver"
	"github.com/michaelact/kakavault/internal/report"
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
//
// Three modes, chosen by which of args.all/namespace/secretName are set:
//   - all:                       discover every namespace/Secret pair in the cluster
//   - namespace, no secretName:  discover every matching Secret in that one namespace
//   - namespace + secretName:    a single, explicit target
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
	ctx := context.Background()

	switch {
	case args.all:
		targets, err := discover.Find(ctx, reader, cfg.NamespacePattern, cfg.SecretNamePattern)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		return runTargets(ctx, cfg, reader, vaultWriter, targets, args.apply, stdout, stderr)

	case args.secretName == "":
		// Namespace given, no specific Secret — discover every matching
		// Secret in just that namespace. Validate the namespace itself
		// against namespace_pattern first, same "fail before touching the
		// cluster" reasoning as the single-target path below.
		nsRe, err := regexp.Compile(cfg.NamespacePattern)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		if !nsRe.MatchString(args.namespace) {
			fmt.Fprintf(stderr, "error: namespace %q does not match namespace_pattern %q\n", args.namespace, cfg.NamespacePattern)
			return 1
		}

		targets, err := discover.FindInNamespace(ctx, reader, args.namespace, cfg.SecretNamePattern)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		return runTargets(ctx, cfg, reader, vaultWriter, targets, args.apply, stdout, stderr)

	default:
		// Validate the namespace/secret-name patterns before touching the
		// cluster at all — a pattern mismatch is a config problem, not
		// something a k8s API failure should mask.
		if _, err := pathresolver.Resolve(cfg.NamespacePattern, cfg.SecretNamePattern, args.namespace, args.secretName); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		failed, err := runOneTarget(ctx, cfg, reader, vaultWriter, args.namespace, args.secretName, args.apply, stdout)
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
}

// runTargets runs each discovered target through runOneTarget, printing a
// header + table per target. Shared by --all and namespace-only discovery.
func runTargets(ctx context.Context, cfg *config.Config, reader *k8sreader.Reader, vaultWriter migrate.VaultWriter, targets []discover.Target, apply bool, stdout, stderr io.Writer) int {
	if len(targets) == 0 {
		fmt.Fprintln(stdout, "No matching Secret(s) found — nothing to do.")
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
