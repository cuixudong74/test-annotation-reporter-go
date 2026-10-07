package report

import (
	"bytes"
	"testing"

	"github.com/cuixudong74/test-annotation-reporter-go/merge"
	"github.com/cuixudong74/test-annotation-reporter-go/result"
)

func TestWriteDeterministic(t *testing.T) {
	m := merge.New()
	_ = m.Add(result.TestResult{Suite: "s", Name: "B", Status: result.StatusFailed, File: "b.go", Line: 2, Message: "x", Shard: "1", Seq: 0})
	_ = m.Add(result.TestResult{Suite: "s", Name: "A", Status: result.StatusPassed, Shard: "1", Seq: 1})
	snap := m.Snapshot()
	var a, b bytes.Buffer
	if err := Write(&a, snap); err != nil {
		t.Fatal(err)
	}
	if err := Write(&b, m.Snapshot()); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Fatal("output not byte-identical")
	}
	want := "total=2 passed=1 failed=1 skipped=0 error=0 rejected=0\nfailed s.B b.go:2 x\n"
	if a.String() != want {
		t.Fatalf("got %q want %q", a.String(), want)
	}
}
