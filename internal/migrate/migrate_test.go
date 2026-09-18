package migrate

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/michaelact/kakavault/internal/config"
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
		"HASH_SECRET":    "hash-value",
	}

	items, err := Plan(testConfig(), secretData, "myrepo-staging", "backend-secret-variables")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })

	if len(items) != 3 {
		t.Fatalf("len(Plan()) = %d, want 3", len(items))
	}
	if items[0].Key != "ENCRYPTION_KEY" || items[0].Classification != "internal" {
		t.Errorf("items[0] = %+v, want Key=ENCRYPTION_KEY Classification=internal", items[0])
	}
	if items[0].SubPath != "myrepo/staging/backend/internal" {
		t.Errorf("items[0].SubPath = %q", items[0].SubPath)
	}
	if items[1].Key != "HASH_SECRET" || items[1].Classification != "internal" {
		t.Errorf("items[1] = %+v, want Key=HASH_SECRET Classification=internal", items[1])
	}
	if items[1].SubPath != items[0].SubPath {
		t.Errorf("items sharing a classification must share a SubPath: items[0]=%q items[1]=%q", items[0].SubPath, items[1].SubPath)
	}
	if items[2].Key != "OPENAI_API_KEY" || items[2].Classification != "third-party" {
		t.Errorf("items[2] = %+v, want Key=OPENAI_API_KEY Classification=third-party", items[2])
	}
	if items[2].SubPath != "myrepo/staging/backend/third-party" {
		t.Errorf("items[2].SubPath = %q", items[2].SubPath)
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
	data       map[string]map[string]string
	failWrite  map[string]bool
	writeCalls map[string]int
}

func newFakeWriter() *fakeWriter {
	return &fakeWriter{
		data:       map[string]map[string]string{},
		failWrite:  map[string]bool{},
		writeCalls: map[string]int{},
	}
}

func (f *fakeWriter) Write(ctx context.Context, subpath string, values map[string]string) error {
	f.writeCalls[subpath]++
	if f.failWrite[subpath] {
		return errors.New("simulated write failure")
	}
	f.data[subpath] = values
	return nil
}

func (f *fakeWriter) Read(ctx context.Context, subpath string) (map[string]string, bool, error) {
	v, ok := f.data[subpath]
	return v, ok, nil
}

func TestApply_AllSucceed(t *testing.T) {
	items := []PlannedItem{
		{Key: "ENCRYPTION_KEY", Classification: "internal", SubPath: "myrepo/staging/backend/internal"},
		{Key: "HASH_SECRET", Classification: "internal", SubPath: "myrepo/staging/backend/internal"},
		{Key: "OPENAI_API_KEY", Classification: "third-party", SubPath: "myrepo/staging/backend/third-party"},
	}
	secretData := map[string]string{"ENCRYPTION_KEY": "enc-value", "HASH_SECRET": "hash-value", "OPENAI_API_KEY": "sk-abc"}
	writer := newFakeWriter()

	results := Apply(context.Background(), writer, items, secretData)

	if len(results) != 3 {
		t.Fatalf("len(Apply()) = %d, want 3", len(results))
	}
	for _, r := range results {
		if r.Status != "written" {
			t.Errorf("result for %s: Status = %q, want %q (err=%v)", r.Key, r.Status, "written", r.Err)
		}
	}
}

func TestApply_GroupsByClassification_OneWritePerGroup(t *testing.T) {
	items := []PlannedItem{
		{Key: "ENCRYPTION_KEY", Classification: "internal", SubPath: "myrepo/staging/backend/internal"},
		{Key: "HASH_SECRET", Classification: "internal", SubPath: "myrepo/staging/backend/internal"},
	}
	secretData := map[string]string{"ENCRYPTION_KEY": "enc-value", "HASH_SECRET": "hash-value"}
	writer := newFakeWriter()

	Apply(context.Background(), writer, items, secretData)

	if got := writer.writeCalls["myrepo/staging/backend/internal"]; got != 1 {
		t.Errorf("Write called %d times for the shared subpath, want 1", got)
	}

	written := writer.data["myrepo/staging/backend/internal"]
	if written["ENCRYPTION_KEY"] != "enc-value" || written["HASH_SECRET"] != "hash-value" {
		t.Errorf("written data = %+v, want both keys present with their values", written)
	}
}

func TestApply_GroupWriteFails_AllKeysInGroupFail_OtherGroupsStillRun(t *testing.T) {
	items := []PlannedItem{
		{Key: "A", Classification: "internal", SubPath: "myrepo/staging/backend/internal"},
		{Key: "B", Classification: "internal", SubPath: "myrepo/staging/backend/internal"},
		{Key: "C", Classification: "third-party", SubPath: "myrepo/staging/backend/third-party"},
	}
	secretData := map[string]string{"A": "value-a", "B": "value-b", "C": "value-c"}
	writer := newFakeWriter()
	writer.failWrite["myrepo/staging/backend/internal"] = true

	results := Apply(context.Background(), writer, items, secretData)

	if len(results) != 3 {
		t.Fatalf("len(Apply()) = %d, want 3", len(results))
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
	if byKey["B"].Status != "write-failed" {
		t.Errorf("B.Status = %q, want %q (shares A's group, same write failure)", byKey["B"].Status, "write-failed")
	}
	if byKey["C"].Status != "written" {
		t.Errorf("C.Status = %q, want %q (a different group's failure must not block it)", byKey["C"].Status, "written")
	}
}

func TestApply_VerifyFailsIfReadBackMismatches(t *testing.T) {
	items := []PlannedItem{
		{Key: "A", Classification: "internal", SubPath: "myrepo/staging/backend/internal"},
	}
	secretData := map[string]string{"A": "expected-value"}

	// A writer whose Write is a no-op, so the pre-seeded (wrong) value
	// already in fakeWriter.data is what Read returns — simulates drift
	// between what was written and what's actually stored, which Apply's
	// read-back should catch.
	writer := newFakeWriter()
	writer.data["myrepo/staging/backend/internal"] = map[string]string{"A": "WRONG_VALUE_ALREADY_PRESENT"}

	results := Apply(context.Background(), &mismatchWriter{fakeWriter: writer}, items, secretData)

	if len(results) != 1 {
		t.Fatalf("len(Apply()) = %d, want 1", len(results))
	}
	if results[0].Status != "verify-failed" {
		t.Errorf("Status = %q, want %q", results[0].Status, "verify-failed")
	}
}

func TestApply_VerifyFailsForJustOneKey_OthersInSameGroupStillVerify(t *testing.T) {
	items := []PlannedItem{
		{Key: "A", Classification: "internal", SubPath: "myrepo/staging/backend/internal"},
		{Key: "B", Classification: "internal", SubPath: "myrepo/staging/backend/internal"},
	}
	secretData := map[string]string{"A": "expected-a", "B": "expected-b"}

	writer := newFakeWriter()
	// Pre-seed with A wrong, B correct — Write is a no-op below, so this
	// pre-seeded state is exactly what Read returns.
	writer.data["myrepo/staging/backend/internal"] = map[string]string{"A": "WRONG", "B": "expected-b"}

	results := Apply(context.Background(), &mismatchWriter{fakeWriter: writer}, items, secretData)

	byKey := map[string]Result{}
	for _, r := range results {
		byKey[r.Key] = r
	}
	if byKey["A"].Status != "verify-failed" {
		t.Errorf("A.Status = %q, want %q", byKey["A"].Status, "verify-failed")
	}
	if byKey["B"].Status != "written" {
		t.Errorf("B.Status = %q, want %q (its own value matched, even though A's in the same group didn't)", byKey["B"].Status, "written")
	}
}

// mismatchWriter wraps fakeWriter but makes Write a no-op, so the
// pre-seeded (wrong) value in fakeWriter.data is what Read returns.
type mismatchWriter struct {
	*fakeWriter
}

func (m *mismatchWriter) Write(ctx context.Context, subpath string, values map[string]string) error {
	return nil // intentionally doesn't store values
}
