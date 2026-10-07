// Package report renders a deterministic, byte-stable text summary of a
// merged snapshot.
package report

import (
	"fmt"
	"io"

	"github.com/cuixudong74/test-annotation-reporter-go/merge"
	"github.com/cuixudong74/test-annotation-reporter-go/result"
)

// Write renders the snapshot in a deterministic order: counts first, then
// failing tests sorted by (suite, name) as provided by the snapshot.
func Write(w io.Writer, s merge.Snapshot) error {
	if _, err := fmt.Fprintf(w, "total=%d passed=%d failed=%d skipped=%d error=%d rejected=%d\n",
		s.Counts.Total(), s.Counts.Passed, s.Counts.Failed,
		s.Counts.Skipped, s.Counts.Errored, s.Counts.Rejected); err != nil {
		return err
	}
	for _, r := range s.Results {
		if r.Status == result.StatusPassed || r.Status == result.StatusSkipped {
			continue
		}
		loc := r.File
		if r.Line > 0 {
			loc = fmt.Sprintf("%s:%d", r.File, r.Line)
		}
		if loc == "" {
			loc = "-"
		}
		if _, err := fmt.Fprintf(w, "%s %s.%s %s %s\n",
			r.Status, r.Suite, r.Name, loc, r.Message); err != nil {
			return err
		}
	}
	return nil
}
