// Package migrate orchestrates classify + pathresolver into a plan, and
// applies that plan against a VaultWriter with per-key error isolation
// and read-back verification.
package migrate

import (
	"context"

	"github.com/michaelact/k2v/internal/classify"
	"github.com/michaelact/k2v/internal/config"
	"github.com/michaelact/k2v/internal/pathresolver"
)

// PlannedItem is one Secret key's classification and destination path,
// before anything has been written.
type PlannedItem struct {
	Key            string
	Classification string
	SubPath        string
}

// Result is a PlannedItem plus its outcome after Apply.
type Result struct {
	PlannedItem
	Status string // "planned", "written", "write-failed", "verify-failed"
	Err    error
}

// VaultWriter is the subset of vaultwriter.Writer that Apply needs —
// defined here so migrate's tests use an in-memory fake instead of a
// real Vault server.
type VaultWriter interface {
	Write(ctx context.Context, subpath, value string) error
	Read(ctx context.Context, subpath string) (string, bool, error)
}

// Plan classifies every key in secretData and builds its destination
// path, without writing anything.
func Plan(cfg *config.Config, secretData map[string]string, namespace, secretName string) ([]PlannedItem, error) {
	resolved, err := pathresolver.Resolve(cfg.NamespacePattern, cfg.SecretNamePattern, namespace, secretName)
	if err != nil {
		return nil, err
	}

	classifier, err := classify.New(cfg.Classification)
	if err != nil {
		return nil, err
	}

	items := make([]PlannedItem, 0, len(secretData))
	for key := range secretData {
		classification := classifier.Classify(key)
		subPath := pathresolver.BuildPath(resolved.Repo, resolved.Environment, resolved.Application, classification, key)
		items = append(items, PlannedItem{Key: key, Classification: classification, SubPath: subPath})
	}

	return items, nil
}

// Apply writes every item to Vault via writer, then reads each one back
// to verify it landed correctly. One item's failure doesn't stop the
// rest — every item in items gets a Result.
func Apply(ctx context.Context, writer VaultWriter, items []PlannedItem, secretData map[string]string) []Result {
	results := make([]Result, 0, len(items))

	for _, item := range items {
		value := secretData[item.Key]

		if err := writer.Write(ctx, item.SubPath, value); err != nil {
			results = append(results, Result{PlannedItem: item, Status: "write-failed", Err: err})
			continue
		}

		readBack, found, err := writer.Read(ctx, item.SubPath)
		if err != nil {
			results = append(results, Result{PlannedItem: item, Status: "verify-failed", Err: err})
			continue
		}
		if !found || readBack != value {
			results = append(results, Result{PlannedItem: item, Status: "verify-failed"})
			continue
		}

		results = append(results, Result{PlannedItem: item, Status: "written"})
	}

	return results
}
