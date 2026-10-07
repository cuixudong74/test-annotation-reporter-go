package ingest

import (
	"strings"
	"testing"

	"github.com/cuixudong74/test-annotation-reporter-go/result"
)

func collect(p Parser, input, shard string) ([]result.TestResult, []*result.RejectError, Stats) {
	var got []result.TestResult
	var rejs []*result.RejectError
	st := p.Parse(strings.NewReader(input), shard,
		func(r result.TestResult) { got = append(got, r) },
		func(e *result.RejectError) { rejs = append(rejs, e) })
	return got, rejs, st
}

func TestJSONLParsesAllStatuses(t *testing.T) {
	in := `{"suite":"s","name":"A","status":"passed"}
{"suite":"s","name":"B","status":"failed","file":"b.go","line":7,"message":"boom"}
{"suite":"s","name":"C","status":"skipped"}
{"suite":"s","name":"D","status":"error","file":"d.go","line":9}
`
	got, rejs, st := collect(jsonlParser{}, in, "sh")
	if len(rejs) != 0 || st.Parsed != 4 {
		t.Fatalf("parsed=%d rejects=%v", st.Parsed, rejs)
	}
	if got[1].Status != result.StatusFailed || got[1].File != "b.go" || got[1].Line != 7 || got[1].Message != "boom" {
		t.Fatalf("bad failed record: %+v", got[1])
	}
	if got[3].Status != result.StatusError {
		t.Fatalf("bad error record: %+v", got[3])
	}
	for i, r := range got {
		if r.Shard != "sh" || r.Seq != uint64(i) {
			t.Fatalf("record %d missing shard/seq: %+v", i, r)
		}
	}
}

func TestJSONLRejectsWithDistinctReasons(t *testing.T) {
	in := `not json at all
{"name":"NoStatus","status":"weird"}
{"status":"passed"}
{"suite":"s","name":"OK","status":"passed"}
`
	got, rejs, st := collect(jsonlParser{}, in, "sh")
	if len(got) != 1 || st.Rejected != 3 {
		t.Fatalf("got=%d rejected=%d", len(got), st.Rejected)
	}
	wantReasons := []result.RejectReason{result.ReasonMalformed, result.ReasonUnknownFormat, result.ReasonMissingField}
	for i, w := range wantReasons {
		if rejs[i].Reason != w {
			t.Fatalf("reject %d reason=%v want %v", i, rejs[i].Reason, w)
		}
	}
}

func TestJSONLTruncatedTailRejected(t *testing.T) {
	in := "{\"suite\":\"s\",\"name\":\"A\",\"status\":\"passed\"}\n{\"suite\":\"s\",\"name\":\"B\",\"stat"
	got, rejs, _ := collect(jsonlParser{}, in, "sh")
	if len(got) != 1 || len(rejs) != 1 || rejs[0].Reason != result.ReasonMalformed {
		t.Fatalf("got=%v rejs=%v", got, rejs)
	}
}

func TestJUnitParsesAndSurvivesCorruption(t *testing.T) {
	in := `<testsuite name="s">
<testcase name="A" classname="pkg"><failure message="nope">stack</failure></testcase>
<testcase name="B" classname="pkg" file="b.go" line="12"><error>exploded</error></testcase>
<testcase name="C" classname="pkg"><skipped/></testcase>
<testcase name="D" classname="pkg"/>
<testcase name="E" classname="pkg"><failure>unclosed
`
	got, rejs, st := collect(junitParser{}, in, "sh")
	if st.Parsed != 4 || len(got) != 4 {
		t.Fatalf("parsed=%d got=%v", st.Parsed, got)
	}
	if got[0].Status != result.StatusFailed || got[0].Message != "nope" {
		t.Fatalf("bad failure: %+v", got[0])
	}
	if got[1].Status != result.StatusError || got[1].File != "b.go" || got[1].Line != 12 || got[1].Message != "exploded" {
		t.Fatalf("bad error: %+v", got[1])
	}
	if got[2].Status != result.StatusSkipped || got[3].Status != result.StatusPassed {
		t.Fatalf("bad statuses: %v %v", got[2].Status, got[3].Status)
	}
	// The corrupt tail must be rejected, not silently dropped.
	if len(rejs) == 0 || rejs[len(rejs)-1].Reason != result.ReasonMalformed {
		t.Fatalf("corrupt tail not reported: %v", rejs)
	}
}

func TestGoTestParsesEventsAndLocation(t *testing.T) {
	in := `{"Action":"run","Package":"p","Test":"TestA"}
{"Action":"output","Package":"p","Test":"TestA","Output":"    a_test.go:42: boom\n"}
{"Action":"fail","Package":"p","Test":"TestA"}
{"Action":"run","Package":"p","Test":"TestB"}
{"Action":"pass","Package":"p","Test":"TestB"}
{"Action":"run","Package":"p","Test":"TestC"}
{"Action":"skip","Package":"p","Test":"TestC"}
{"Action":"run","Package":"p","Test":"TestD"}
`
	got, rejs, st := collect(gotestParser{}, in, "sh")
	if len(rejs) != 0 {
		t.Fatalf("unexpected rejects: %v", rejs)
	}
	if st.Parsed != 4 {
		t.Fatalf("parsed=%d got=%v", st.Parsed, got)
	}
	if got[0].Status != result.StatusFailed || got[0].File != "a_test.go" || got[0].Line != 42 || got[0].Message != "boom" {
		t.Fatalf("bad failed test: %+v", got[0])
	}
	// TestD never terminated: stream interrupted -> marked as error.
	if got[3].Name != "TestD" || got[3].Status != result.StatusError {
		t.Fatalf("interrupted test not marked: %+v", got[3])
	}
}

func TestGoTestRejectsMalformedAndUnknown(t *testing.T) {
	in := "{bad json\n" +
		`{"Action":"explode","Package":"p","Test":"T"}` + "\n"
	_, rejs, st := collect(gotestParser{}, in, "sh")
	if st.Rejected != 2 || rejs[0].Reason != result.ReasonMalformed || rejs[1].Reason != result.ReasonUnknownFormat {
		t.Fatalf("rejs=%v", rejs)
	}
}

func TestForFormatRejectsUnsupported(t *testing.T) {
	if _, err := ForFormat("nope"); err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if _, err := ForFormat("junit"); err != nil {
		t.Fatal(err)
	}
}
