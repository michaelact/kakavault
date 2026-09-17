// Package report prints the dry-run and apply result tables to an
// io.Writer (stdout in production, a buffer in tests).
package report

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/michaelact/k2v/internal/migrate"
)

// PrintPlanned prints the dry-run table: what WOULD be written.
func PrintPlanned(w io.Writer, items []migrate.PlannedItem) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tCLASSIFICATION\tVAULT PATH")
	for _, item := range items {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", item.Key, item.Classification, item.SubPath)
	}
	tw.Flush()
}

// PrintResults prints the post-apply table and returns how many items
// failed (write-failed or verify-failed), for the caller to decide the
// process exit code.
func PrintResults(w io.Writer, results []migrate.Result) (failed int) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tCLASSIFICATION\tVAULT PATH\tSTATUS\tERROR")
	for _, r := range results {
		errText := ""
		if r.Err != nil {
			errText = r.Err.Error()
		}
		if r.Status == "write-failed" || r.Status == "verify-failed" {
			failed++
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Key, r.Classification, r.SubPath, r.Status, errText)
	}
	tw.Flush()
	return failed
}
