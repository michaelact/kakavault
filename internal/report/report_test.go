package report

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/michaelact/k2v/internal/migrate"
)

func TestPrintPlanned(t *testing.T) {
	items := []migrate.PlannedItem{
		{Key: "ENCRYPTION_KEY", Classification: "internal", SubPath: "myrepo/staging/backend/internal/ENCRYPTION_KEY"},
	}

	var buf bytes.Buffer
	PrintPlanned(&buf, items)

	out := buf.String()
	for _, want := range []string{"ENCRYPTION_KEY", "internal", "myrepo/staging/backend/internal/ENCRYPTION_KEY"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

func TestPrintResults_ReturnsFailedCount(t *testing.T) {
	results := []migrate.Result{
		{PlannedItem: migrate.PlannedItem{Key: "A"}, Status: "written"},
		{PlannedItem: migrate.PlannedItem{Key: "B"}, Status: "write-failed", Err: errors.New("boom")},
		{PlannedItem: migrate.PlannedItem{Key: "C"}, Status: "verify-failed"},
	}

	var buf bytes.Buffer
	failed := PrintResults(&buf, results)

	if failed != 2 {
		t.Errorf("PrintResults() failed count = %d, want 2", failed)
	}

	out := buf.String()
	if !strings.Contains(out, "boom") {
		t.Errorf("output should include the failure's error message; got:\n%s", out)
	}
}

func TestPrintResults_AllSucceed(t *testing.T) {
	results := []migrate.Result{
		{PlannedItem: migrate.PlannedItem{Key: "A"}, Status: "written"},
	}

	var buf bytes.Buffer
	failed := PrintResults(&buf, results)

	if failed != 0 {
		t.Errorf("PrintResults() failed count = %d, want 0", failed)
	}
}
