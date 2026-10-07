package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeShards(t *testing.T) (dir string, files []string) {
	t.Helper()
	dir = t.TempDir()
	shards := map[string]string{
		"shard-a.jsonl": "{\"suite\":\"s\",\"name\":\"T1\",\"status\":\"passed\"}\n" +
			"{\"suite\":\"s\",\"name\":\"T2\",\"status\":\"failed\",\"file\":\"a.go\",\"line\":3,\"message\":\"boom\"}\n",
		"shard-b.jsonl": "{\"suite\":\"s\",\"name\":\"T1\",\"status\":\"failed\",\"file\":\"b.go\",\"line\":9,\"message\":\"flaky\"}\n" +
			"garbage line\n" +
			"{\"suite\":\"s\",\"name\":\"T3\",\"status\":\"skipped\"}\n",
	}
	for name, content := range shards {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	return dir, files
}

func TestEndToEndDeterministic(t *testing.T) {
	_, files := writeShards(t)
	args := append([]string{"-format", "jsonl", "-platform", "github"}, files...)

	runOnce := func() (string, int) {
		var out, errBuf bytes.Buffer
		code := run(args, &out, &errBuf)
		return out.String(), code
	}
	first, code1 := runOnce()
	second, code2 := runOnce()
	if first != second || code1 != code2 {
		t.Fatalf("not byte-identical across runs:\n%q\nvs\n%q", first, second)
	}
	if code1 != 1 { // failures present -> nonzero exit
		t.Fatalf("exit=%d want 1", code1)
	}
	// T1 last result comes from shard-b (failed), T2 failed, T3 skipped.
	if !strings.Contains(first, "total=3 passed=0 failed=2 skipped=1 error=0 rejected=1") {
		t.Fatalf("bad counts line: %q", first)
	}
	if !strings.Contains(first, "::error file=b.go,line=9::s.T1: flaky") {
		t.Fatalf("missing T1 annotation: %q", first)
	}
	if !strings.Contains(first, "::error file=a.go,line=3::s.T2: boom") {
		t.Fatalf("missing T2 annotation: %q", first)
	}
}

func TestEndToEndAnnotationCap(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for i := 0; i < 10; i++ {
		b.WriteString(`{"suite":"s","name":"F` + string(rune('A'+i)) + `","status":"failed","file":"f.go","line":1}`)
		b.WriteByte('\n')
	}
	p := filepath.Join(dir, "in.jsonl")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := run([]string{"-format", "jsonl", "-platform", "azure", "-max-annotations", "3", p}, &out, &out)
	if code != 1 {
		t.Fatalf("exit=%d", code)
	}
	lines := strings.Count(strings.TrimSpace(out.String()), "\n") + 1
	// 1 counts line + 10 failure lines + 3 annotations + 1 summary
	if lines != 15 {
		t.Fatalf("got %d lines:\n%s", lines, out.String())
	}
	if !strings.Contains(out.String(), "7 further annotation(s) omitted") {
		t.Fatalf("missing omission summary:\n%s", out.String())
	}
}
