package merge

import (
	"fmt"
	"testing"

	"github.com/cuixudong74/test-annotation-reporter-go/result"
)

func res(shard string, seq uint64, name string, st result.Status) result.TestResult {
	return result.TestResult{Suite: "pkg", Name: name, Status: st, Shard: shard, Seq: seq}
}

func TestOutOfOrderShardsMergeLikeSerial(t *testing.T) {
	// Serial reference: shard A seq 0..2, then shard B seq 0..2, applied in
	// that order; last write per test wins.
	serial := New()
	for _, r := range []result.TestResult{
		res("A", 0, "T1", result.StatusPassed),
		res("A", 1, "T2", result.StatusPassed),
		res("A", 2, "T3", result.StatusPassed),
		res("B", 0, "T1", result.StatusFailed), // T1 re-ran on B and failed
		res("B", 1, "T4", result.StatusSkipped),
		res("B", 2, "T5", result.StatusError),
	} {
		if err := serial.Add(r); err != nil {
			t.Fatal(err)
		}
	}

	// Parallel: same records interleaved across shards in arrival order
	// that differs from shard-local order of submission.
	par := New()
	for _, r := range []result.TestResult{
		res("A", 0, "T1", result.StatusPassed),
		res("B", 0, "T1", result.StatusFailed),
		res("A", 1, "T2", result.StatusPassed),
		res("B", 1, "T4", result.StatusSkipped),
		res("A", 2, "T3", result.StatusPassed),
		res("B", 2, "T5", result.StatusError),
	} {
		if err := par.Add(r); err != nil {
			t.Fatal(err)
		}
	}

	a, b := serial.Snapshot(), par.Snapshot()
	if a.Counts != b.Counts {
		t.Fatalf("counts differ: serial=%+v parallel=%+v", a.Counts, b.Counts)
	}
	if fmt.Sprintf("%v", a.Results) != fmt.Sprintf("%v", b.Results) {
		t.Fatalf("results differ:\nserial=%v\nparallel=%v", a.Results, b.Results)
	}
	want := Counts{Passed: 2, Failed: 1, Skipped: 1, Errored: 1}
	if a.Counts != want {
		t.Fatalf("counts=%+v want %+v", a.Counts, want)
	}
}

func TestDuplicateAndResumedRecordsRejected(t *testing.T) {
	m := New()
	if err := m.Add(res("A", 0, "T1", result.StatusPassed)); err != nil {
		t.Fatal(err)
	}
	// Stream interrupted and re-ingested from the start: replays of
	// already-seen sequences must be rejected as duplicates.
	for _, seq := range []uint64{0, 0} {
		err := m.Add(res("A", seq, "T1", result.StatusFailed))
		if err == nil {
			t.Fatalf("seq %d: expected duplicate rejection", seq)
		}
		re, ok := err.(*result.RejectError)
		if !ok || re.Reason != result.ReasonDuplicate {
			t.Fatalf("seq %d: got %v, want duplicate RejectError", seq, err)
		}
	}
	// The genuinely new record after the interruption point is accepted
	// and, as the latest execution, replaces the earlier result.
	if err := m.Add(res("A", 1, "T1", result.StatusFailed)); err != nil {
		t.Fatal(err)
	}
	snap := m.Snapshot()
	if snap.Counts.Total() != 1 || snap.Counts.Failed != 1 {
		t.Fatalf("wrong counts after resume: %+v", snap.Counts)
	}
}

func TestSnapshotDeterministic(t *testing.T) {
	build := func() Snapshot {
		m := New()
		for i := 0; i < 50; i++ {
			_ = m.Add(res("s", uint64(i), fmt.Sprintf("T%02d", i), result.StatusFailed))
		}
		return m.Snapshot()
	}
	a, b := build(), build()
	if fmt.Sprintf("%v", a.Results) != fmt.Sprintf("%v", b.Results) {
		t.Fatal("snapshots not deterministic")
	}
	for i := 1; i < len(a.Results); i++ {
		if a.Results[i-1].Name > a.Results[i].Name {
			t.Fatal("snapshot not sorted")
		}
	}
}
