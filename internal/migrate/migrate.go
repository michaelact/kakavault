// Package migrate orchestrates classify + pathresolver into a plan, and
// applies that plan against a VaultWriter. Keys sharing a classification
// share one Vault secret — Apply groups by SubPath so each group gets a
// single write, then reads it back to verify every key in it.
package migrate

import (
	"context"

	"github.com/michaelact/kakavault/internal/classify"
	"github.com/michaelact/kakavault/internal/config"
	"github.com/michaelact/kakavault/internal/pathresolver"
)

// PlannedItem is one Secret key's classification and destination path,
// before anything has been written. Items sharing a classification share
// a SubPath — they land in the same Vault secret.
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
	Write(ctx context.Context, subpath string, values map[string]string) error
	Read(ctx context.Context, subpath string) (map[string]string, bool, error)
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
		subPath := pathresolver.BuildPath(resolved.Repo, resolved.Environment, resolved.Application, classification)
		items = append(items, PlannedItem{Key: key, Classification: classification, SubPath: subPath})
	}

	return items, nil
}

// Apply groups items by SubPath, writes each group as one Vault secret
// (every key in the group as a field), then reads each group back and
// verifies every key individually. One group's write failure doesn't
// stop other groups — every item in items still gets a Result.
func Apply(ctx context.Context, writer VaultWriter, items []PlannedItem, secretData map[string]string) []Result {
	groups := groupBySubPath(items)

	results := make([]Result, 0, len(items))
	for _, group := range groups {
		values := make(map[string]string, len(group.items))
		for _, item := range group.items {
			values[item.Key] = secretData[item.Key]
		}

		if err := writer.Write(ctx, group.subPath, values); err != nil {
			for _, item := range group.items {
				results = append(results, Result{PlannedItem: item, Status: "write-failed", Err: err})
			}
			continue
		}

		readBack, found, err := writer.Read(ctx, group.subPath)
		if err != nil {
			for _, item := range group.items {
				results = append(results, Result{PlannedItem: item, Status: "verify-failed", Err: err})
			}
			continue
		}

		for _, item := range group.items {
			if !found || readBack[item.Key] != secretData[item.Key] {
				results = append(results, Result{PlannedItem: item, Status: "verify-failed"})
				continue
			}
			results = append(results, Result{PlannedItem: item, Status: "written"})
		}
	}

	return results
}

type subPathGroup struct {
	subPath string
	items   []PlannedItem
}

// groupBySubPath preserves each group's first-seen order, so Apply's
// results come back in a stable, deterministic sequence for a given
// items order.
func groupBySubPath(items []PlannedItem) []subPathGroup {
	index := map[string]int{}
	var groups []subPathGroup

	for _, item := range items {
		i, ok := index[item.SubPath]
		if !ok {
			i = len(groups)
			index[item.SubPath] = i
			groups = append(groups, subPathGroup{subPath: item.SubPath})
		}
		groups[i].items = append(groups[i].items, item)
	}

	return groups
}
