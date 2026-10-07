// Package merge combines results arriving from many shards into one
// consistent report. Memory stays bounded by the number of distinct test
// cases plus the number of shards, not by the number of input records.
package merge

import (
	"sort"
	"sync"

	"github.com/cuixudong74/test-annotation-reporter-go/result"
)

// Merger deduplicates records and keeps the latest result per test case.
// It is safe for concurrent use.
type Merger struct {
	mu       sync.Mutex
	latest   map[string]result.TestResult // test key -> latest result
	next     map[string]uint64            // shard -> lowest Seq still accepted
	rejected uint64
}

// New creates an empty Merger.
func New() *Merger {
	return &Merger{
		latest: make(map[string]result.TestResult),
		next:   make(map[string]uint64),
	}
}

// Add ingests one result. Records whose Seq is not greater than the shard
// watermark are rejected as duplicates, so an interrupted stream that is
// re-ingested never double-counts. Among shards, the record accepted last
// wins for a given test case, matching serial execution order.
func (m *Merger) Add(r result.TestResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.Seq < m.next[r.Shard] {
		return &result.RejectError{
			Reason: result.ReasonDuplicate,
			Detail: "shard " + r.Shard + " already past this sequence",
		}
	}
	m.next[r.Shard] = r.Seq + 1
	m.latest[r.Key()] = r
	return nil
}

// Reject records a refused input record for accounting.
func (m *Merger) Reject() {
	m.mu.Lock()
	m.rejected++
	m.mu.Unlock()
}

// Counts is a tally by status.
type Counts struct {
	Passed, Failed, Skipped, Errored, Rejected uint64
}

// Total returns the number of merged test cases.
func (c Counts) Total() uint64 { return c.Passed + c.Failed + c.Skipped + c.Errored }

// Snapshot is a consistent, deterministically ordered view of the report.
type Snapshot struct {
	Counts  Counts
	Results []result.TestResult // sorted by (Suite, Name)
}

// Snapshot returns a consistent view; safe to call while ingestion runs.
// Results are sorted so repeated runs over the same input are byte-stable.
func (m *Merger) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := Snapshot{
		Counts:  Counts{Rejected: m.rejected},
		Results: make([]result.TestResult, 0, len(m.latest)),
	}
	for _, r := range m.latest {
		s.Results = append(s.Results, r)
		switch r.Status {
		case result.StatusPassed:
			s.Counts.Passed++
		case result.StatusFailed:
			s.Counts.Failed++
		case result.StatusSkipped:
			s.Counts.Skipped++
		case result.StatusError:
			s.Counts.Errored++
		}
	}
	sort.Slice(s.Results, func(i, j int) bool {
		a, b := s.Results[i], s.Results[j]
		if a.Suite != b.Suite {
			return a.Suite < b.Suite
		}
		return a.Name < b.Name
	})
	return s
}
