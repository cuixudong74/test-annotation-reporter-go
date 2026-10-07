package reporter

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// --- 归并语义 ---

func TestMergeOutOfOrderShardsMatchesSerial(t *testing.T) {
	// 构造 4 个分片、每片 50 条结果。
	var shards [][]TestResult
	for s := 0; s < 4; s++ {
		var shard []TestResult
		for i := 0; i < 50; i++ {
			st := StatusPassed
			if i%7 == 0 {
				st = StatusFailed
			}
			shard = append(shard, TestResult{
				ID: fmt.Sprintf("pkg.Test%02d", i), Status: st,
				Shard: fmt.Sprintf("shard-%d", s),
			})
		}
		shards = append(shards, shard)
	}

	// 串行基准：分片 0..3 顺序摄入。
	serial := NewMerger()
	for _, shard := range shards {
		for _, r := range shard {
			serial.Add(r)
		}
	}
	want := reportBytes(t, serial.Snapshot())

	// 乱序并行：固定种子随机交错。
	rng := rand.New(rand.NewSource(42))
	merged := NewMerger()
	done := make(chan []TestResult, 4)
	for _, shard := range shards {
		s := shard
		go func() {
			for i := len(s) - 1; i > 0; i-- { // 片内也打乱
				j := rng.Intn(i + 1)
				s[i], s[j] = s[j], s[i]
			}
			done <- s
		}()
	}
	for i := 0; i < 4; i++ {
		for _, r := range <-done {
			merged.Add(r)
		}
	}
	got := reportBytes(t, merged.Snapshot())
	if !bytes.Equal(want, got) {
		t.Fatalf("out-of-order merge differs from serial:\nserial:\n%s\nmerged:\n%s", want, got)
	}
}

func TestMergeDuplicateLatestWins(t *testing.T) {
	m := NewMerger()
	m.Add(TestResult{ID: "p.T", Status: StatusFailed, Message: "old"})
	m.Add(TestResult{ID: "p.T", Status: StatusPassed})
	rep := m.Snapshot()
	if rep.Counts[StatusFailed] != 0 || rep.Counts[StatusPassed] != 1 {
		t.Fatalf("latest result should win, got %+v", rep.Counts)
	}
}

func TestSnapshotDeterministicAcrossRuns(t *testing.T) {
	build := func() []byte {
		m := NewMerger()
		for i := 99; i >= 0; i-- {
			m.Add(TestResult{ID: fmt.Sprintf("p.T%03d", i), Status: Status(i % 4)})
		}
		return reportBytes(t, m.Snapshot())
	}
	if a, b := build(), build(); !bytes.Equal(a, b) {
		t.Fatal("snapshots not byte-identical")
	}
}

// --- 解析与非法输入 ---

func TestParseGoTestJSON(t *testing.T) {
	input := `{"Action":"run","Package":"p","Test":"TestA"}
{"Action":"output","Package":"p","Test":"TestA","Output":"    foo_test.go:12: boom\n"}
{"Action":"fail","Package":"p","Test":"TestA"}
{"Action":"pass","Package":"p","Test":"TestB"}
{"Action":"skip","Package":"p","Test":"TestC"}
not-json{{{
{"Action":"mystery","Package":"p"}
{"Action":"fail","Package":"p"}
`
	recs := parseAll(t, FormatGoTestJSON, "s1", input)
	var results []TestResult
	var rejects []*RejectError
	for _, r := range recs {
		if r.Reject != nil {
			rejects = append(rejects, r.Reject)
		} else {
			results = append(results, *r.Result)
		}
	}
	if len(results) != 4 {
		t.Fatalf("want 4 results, got %d: %+v", len(results), results)
	}
	fail := results[0]
	if fail.Status != StatusFailed || fail.File != "foo_test.go" || fail.Line != 12 {
		t.Fatalf("bad failed result: %+v", fail)
	}
	if results[3].Status != StatusError { // 包级 fail
		t.Fatalf("package-level fail should be error, got %+v", results[3])
	}
	if len(rejects) != 2 {
		t.Fatalf("want 2 rejects, got %d", len(rejects))
	}
	if rejects[0].Reason != RejectMalformed || rejects[1].Reason != RejectUnknownFormat {
		t.Fatalf("bad reject reasons: %v, %v", rejects[0].Reason, rejects[1].Reason)
	}
}

func TestParseJUnitXML(t *testing.T) {
	input := `<testsuite name="s">
<testcase classname="pkg" name="TestOK"/>
<testcase classname="pkg" name="TestBad"><failure message="assert failed">    bar_test.go:34: want 1 got 2</failure></testcase>
<testcase classname="pkg" name="TestErr" file="baz.go" line="9"><error message="panic"/></testcase>
<testcase classname="pkg" name="TestSkip"><skipped/></testcase>
<testcase/>
</testsuite>`
	recs := parseAll(t, FormatJUnitXML, "s2", input)
	var results []TestResult
	var rejects []*RejectError
	for _, r := range recs {
		if r.Reject != nil {
			rejects = append(rejects, r.Reject)
		} else {
			results = append(results, *r.Result)
		}
	}
	if len(results) != 4 {
		t.Fatalf("want 4 results, got %d: %+v", len(results), results)
	}
	if results[1].File != "bar_test.go" || results[1].Line != 34 {
		t.Fatalf("file/line not extracted from failure text: %+v", results[1])
	}
	if results[2].Status != StatusError || results[2].File != "baz.go" || results[2].Line != 9 {
		t.Fatalf("bad error result: %+v", results[2])
	}
	if len(rejects) != 1 || rejects[0].Reason != RejectMissingID {
		t.Fatalf("want 1 missing-id reject, got %+v", rejects)
	}
}

func TestParseCorruptedStreamContinues(t *testing.T) {
	// 部分写入（最后一行截断）不应影响已解析的记录。
	input := `{"Action":"pass","Package":"p","Test":"A"}
{"Action":"pass","Package":"p","Test":"B"}
{"Action":"pass","Package":"p","Tes`
	recs := parseAll(t, FormatGoTestJSON, "s", input)
	var ok, rejected int
	for _, r := range recs {
		if r.Reject != nil {
			rejected++
		} else {
			ok++
		}
	}
	if ok != 2 || rejected != 1 {
		t.Fatalf("want 2 ok + 1 rejected, got %d ok %d rejected", ok, rejected)
	}
}

func TestIngestResumeNoDoubleCount(t *testing.T) {
	m := NewMerger()
	run := func() {
		ch := make(chan Record, 2)
		ch <- Record{Result: &TestResult{ID: "p.A", Status: StatusFailed}}
		ch <- Record{Result: &TestResult{ID: "p.B", Status: StatusPassed}}
		close(ch)
		m.Ingest(ch, nil)
	}
	run()
	run() // 中断后重放同一批数据
	rep := m.Snapshot()
	if len(rep.Results) != 2 {
		t.Fatalf("re-ingest must not double count, got %d results", len(rep.Results))
	}
}

// --- 注解 ---

func sampleReport() Report {
	m := NewMerger()
	m.Add(TestResult{ID: "p.T1", Status: StatusFailed, File: "a,b:c.go", Line: 3,
		Message: "100% bad\nnewline ::inject [x];"})
	m.Add(TestResult{ID: "p.T2", Status: StatusError, File: "err.go", Line: 1, Message: "panic"})
	m.Add(TestResult{ID: "p.T3", Status: StatusPassed})
	return m.Snapshot()
}

func TestAnnotateGitHubEscaping(t *testing.T) {
	var buf bytes.Buffer
	if err := (Annotator{Platform: PlatformGitHub}).WriteAnnotations(sampleReport(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "::error file=a%2Cb%3Ac.go,line=3::100%25 bad%0Anewline ::inject [x];") {
		t.Fatalf("github escaping wrong:\n%s", out)
	}
	if strings.Contains(out, "a,b:c.go") {
		t.Fatal("unescaped property chars leaked")
	}
}

func TestAnnotateAzureEscaping(t *testing.T) {
	var buf bytes.Buffer
	if err := (Annotator{Platform: PlatformAzure}).WriteAnnotations(sampleReport(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "##vso[task.logissue type=error;sourcepath=a,b:c.go;linenumber=3;]") {
		t.Fatalf("azure header wrong:\n%s", out)
	}
	if !strings.Contains(out, "100%AZP25 bad%0Anewline ::inject [x%5D%3B") {
		t.Fatalf("azure message escaping wrong:\n%s", out)
	}
}

func TestAnnotateGitLabJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := (Annotator{Platform: PlatformGitLab}).WriteAnnotations(sampleReport(), &buf); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 annotations, got %d", len(lines))
	}
	if !strings.Contains(buf.String(), `"message":"panic"`) {
		t.Fatalf("gitlab json wrong: %s", buf.String())
	}
}

func TestAnnotateCapAndDedupe(t *testing.T) {
	m := NewMerger()
	for i := 0; i < 10; i++ {
		m.Add(TestResult{ID: fmt.Sprintf("p.T%d", i), Status: StatusFailed,
			File: "f.go", Line: 1, Message: "same cause"}) // 同一位置同一原因
	}
	for i := 0; i < 5; i++ {
		m.Add(TestResult{ID: fmt.Sprintf("p.U%d", i), Status: StatusFailed,
			File: fmt.Sprintf("g%d.go", i), Line: i, Message: "distinct"})
	}
	rep := m.Snapshot()

	var buf bytes.Buffer
	ann := Annotator{Platform: PlatformGitHub, Max: 3}
	if err := ann.WriteAnnotations(rep, &buf); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 { // 3 条注解 + 1 条聚合说明
		t.Fatalf("want 4 lines, got %d:\n%s", len(lines), buf.String())
	}
	if !strings.Contains(lines[3], "3 annotation(s) omitted") {
		t.Fatalf("missing omitted summary: %s", lines[3])
	}
	// 同一位置同一原因只产出一次：去重后共 6 条，上限 3，省略 3。
	if strings.Count(buf.String(), "same cause") != 1 {
		t.Fatal("duplicate annotation for same location+cause")
	}
}

func TestIncrementalAnnotatorNoDuplicates(t *testing.T) {
	ia := NewIncrementalAnnotator(Annotator{Platform: PlatformGitHub})
	var buf bytes.Buffer
	res := TestResult{ID: "p.T", Status: StatusFailed, File: "f.go", Line: 1, Message: "boom"}
	for i := 0; i < 3; i++ { // 模拟中断后重复摄入
		wrote, err := ia.Add(res, &buf)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && !wrote {
			t.Fatal("first occurrence should be annotated")
		}
		if i > 0 && wrote {
			t.Fatal("duplicate annotation emitted")
		}
	}
	if strings.Count(buf.String(), "::error") != 1 {
		t.Fatalf("want exactly 1 annotation, got:\n%s", buf.String())
	}
}

// --- 辅助 ---

func reportBytes(t *testing.T, rep Report) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := rep.WriteText(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func parseAll(t *testing.T, f Format, shard, input string) []Record {
	t.Helper()
	ch := make(chan Record, 1024)
	if err := ParseStream(f, shard, strings.NewReader(input), ch); err != nil {
		t.Fatal(err)
	}
	close(ch)
	var out []Record
	for r := range ch {
		out = append(out, r)
	}
	return out
}
