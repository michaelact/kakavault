package migrate

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/michaelact/k2v/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		NamespacePattern:  `^(?P<repo>.+)-(?P<environment>[a-z0-9]+)$`,
		SecretNamePattern: `^(?P<application>.+)-secret-variables$`,
		Vault:             config.VaultConfig{Mount: "default", KVVersion: 2},
		Classification: config.Classification{
			Rules: []config.ClassificationRule{
				{Pattern: "_API_KEY$", Classification: "third-party"},
				{Pattern: ".*", Classification: "internal"},
			},
		},
	}
}

func TestPlan_OK(t *testing.T) {
	secretData := map[string]string{
		"OPENAI_API_KEY": "sk-abc",
		"ENCRYPTION_KEY": "enc-value",
	}

	items, err := Plan(testConfig(), secretData, "myrepo-staging", "backend-secret-variables")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })

	if len(items) != 2 {
		t.Fatalf("len(Plan()) = %d, want 2", len(items))
	}
	if items[0].Key != "ENCRYPTION_KEY" || items[0].Classification != "internal" {
		t.Errorf("items[0] = %+v, want Key=ENCRYPTION_KEY Classification=internal", items[0])
	}
	if items[0].SubPath != "myrepo/staging/backend/internal/ENCRYPTION_KEY" {
		t.Errorf("items[0].SubPath = %q", items[0].SubPath)
	}
	if items[1].Key != "OPENAI_API_KEY" || items[1].Classification != "third-party" {
		t.Errorf("items[1] = %+v, want Key=OPENAI_API_KEY Classification=third-party", items[1])
	}
}

func TestPlan_NamespaceMismatch(t *testing.T) {
	_, err := Plan(testConfig(), map[string]string{"X": "y"}, "badnamespace", "backend-secret-variables")
	if err == nil {
		t.Fatal("Plan() error = nil, want an error for a namespace that doesn't match the pattern")
	}
}

// fakeWriter is an in-memory VaultWriter for Apply's tests.
type fakeWriter struct {
	data      map[string]string
	failWrite map[string]bool
}

func newFakeWriter() *fakeWriter {
	return &fakeWriter{data: map[string]string{}, failWrite: map[string]bool{}}
}

func (f *fakeWriter) Write(ctx context.Context, subpath, value string) error {
	if f.failWrite[subpath] {
		return errors.New("simulated write failure")
	}
	f.data[subpath] = value
	return nil
}

func (f *fakeWriter) Read(ctx context.Context, subpath string) (string, bool, error) {
	v, ok := f.data[subpath]
	return v, ok, nil
}

func TestApply_AllSucceed(t *testing.T) {
	items := []PlannedItem{
		{Key: "ENCRYPTION_KEY", Classification: "internal", SubPath: "myrepo/staging/backend/internal/ENCRYPTION_KEY"},
		{Key: "OPENAI_API_KEY", Classification: "third-party", SubPath: "myrepo/staging/backend/third-party/OPENAI_API_KEY"},
	}
	secretData := map[string]string{"ENCRYPTION_KEY": "enc-value", "OPENAI_API_KEY": "sk-abc"}
	writer := newFakeWriter()

	results := Apply(context.Background(), writer, items, secretData)

	if len(results) != 2 {
		t.Fatalf("len(Apply()) = %d, want 2", len(results))
	}
	for _, r := range results {
		if r.Status != "written" {
			t.Errorf("result for %s: Status = %q, want %q (err=%v)", r.Key, r.Status, "written", r.Err)
		}
	}
}

func TestApply_OneWriteFails_OthersStillRun(t *testing.T) {
	items := []PlannedItem{
		{Key: "A", Classification: "internal", SubPath: "myrepo/staging/backend/internal/A"},
		{Key: "B", Classification: "internal", SubPath: "myrepo/staging/backend/internal/B"},
	}
	secretData := map[string]string{"A": "value-a", "B": "value-b"}
	writer := newFakeWriter()
	writer.failWrite["myrepo/staging/backend/internal/A"] = true

	results := Apply(context.Background(), writer, items, secretData)

	if len(results) != 2 {
		t.Fatalf("len(Apply()) = %d, want 2", len(results))
	}

	byKey := map[string]Result{}
	for _, r := range results {
		byKey[r.Key] = r
	}

	if byKey["A"].Status != "write-failed" {
		t.Errorf("A.Status = %q, want %q", byKey["A"].Status, "write-failed")
	}
	if byKey["A"].Err == nil {
		t.Error("A.Err = nil, want the simulated write error")
	}
	if byKey["B"].Status != "written" {
		t.Errorf("B.Status = %q, want %q (a failure on A must not block B)", byKey["B"].Status, "written")
	}
}

func TestApply_VerifyFailsIfReadBackMismatches(t *testing.T) {
	items := []PlannedItem{
		{Key: "A", Classification: "internal", SubPath: "myrepo/staging/backend/internal/A"},
	}
	secretData := map[string]string{"A": "expected-value"}

	// A writer whose Write is a no-op, so the pre-seeded (wrong) value
	// already in fakeWriter.data is what Read returns — simulates drift
	// between what was written and what's actually stored, which Apply's
	// read-back should catch.
	writer := newFakeWriter()
	writer.data["myrepo/staging/backend/internal/A"] = "WRONG_VALUE_ALREADY_PRESENT"

	results := Apply(context.Background(), &mismatchWriter{fakeWriter: writer}, items, secretData)

	if len(results) != 1 {
		t.Fatalf("len(Apply()) = %d, want 1", len(results))
	}
	if results[0].Status != "verify-failed" {
		t.Errorf("Status = %q, want %q", results[0].Status, "verify-failed")
	}
}

// mismatchWriter wraps fakeWriter but makes Write a no-op, so the
// pre-seeded (wrong) value in fakeWriter.data is what Read returns.
type mismatchWriter struct {
	*fakeWriter
}

func (m *mismatchWriter) Write(ctx context.Context, subpath, value string) error {
	return nil // intentionally doesn't store value
}
