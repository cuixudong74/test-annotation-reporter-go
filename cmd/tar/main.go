// Command tar (test annotation reporter) ingests test results from
// multiple shards, merges them and emits a deterministic report plus CI
// annotations.
//
// Usage:
//
//	tar -format jsonl -platform github [-max-annotations 50] [files...]
//
// With no files, input is read from stdin (shard "stdin"). Each file is
// treated as one shard, so re-ingesting a file after an interruption does
// not double-count.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/cuixudong74/test-annotation-reporter-go/annotate"
	"github.com/cuixudong74/test-annotation-reporter-go/ingest"
	"github.com/cuixudong74/test-annotation-reporter-go/merge"
	"github.com/cuixudong74/test-annotation-reporter-go/report"
	"github.com/cuixudong74/test-annotation-reporter-go/result"
)

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tar", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "jsonl", "input format: jsonl|junit|gotest")
	platform := fs.String("platform", "github", "annotation platform: github|gitlab|azure")
	maxAnns := fs.Int("max-annotations", 50, "annotation cap; overflow is summarized")
	shard := fs.String("shard", "stdin", "shard id when reading from stdin")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	parser, err := ingest.ForFormat(*format)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	renderer, err := annotate.ForPlatform(*platform)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	m := merge.New()
	emit := func(r result.TestResult) {
		if err := m.Add(r); err != nil {
			m.Reject()
		}
	}
	onReject := func(*result.RejectError) { m.Reject() }

	ingestOne := func(r io.Reader, shardID string) {
		parser.Parse(r, shardID, emit, onReject)
	}
	files := fs.Args()
	if len(files) == 0 {
		ingestOne(os.Stdin, *shard)
	}
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			fmt.Fprintf(stderr, "open %s: %v\n", f, err)
			return 1
		}
		ingestOne(fh, f)
		fh.Close()
	}

	snap := m.Snapshot()
	if err := report.Write(stdout, snap); err != nil {
		fmt.Fprintf(stderr, "write report: %v\n", err)
		return 1
	}
	anns := annotate.Limit(annotate.FromResults(snap.Results), *maxAnns)
	for _, a := range anns {
		fmt.Fprintln(stdout, renderer.Render(a))
	}
	if snap.Counts.Failed > 0 || snap.Counts.Errored > 0 {
		return 1
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
