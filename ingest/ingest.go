// Package ingest parses multiple test result formats into result.TestResult
// streams. Corrupt records are rejected with a typed reason, never dropped
// silently.
package ingest

import (
	"io"

	"github.com/cuixudong74/test-annotation-reporter-go/result"
)

// Stats summarizes one ingestion run.
type Stats struct {
	Parsed   uint64
	Rejected uint64
}

// Parser turns an input stream into test results. Rejected records are
// reported through onReject and parsing continues where possible.
type Parser interface {
	Parse(r io.Reader, shard string, emit func(result.TestResult), onReject func(*result.RejectError)) Stats
}

// ForFormat returns a parser for the named format, or an error if the
// format is unsupported.
func ForFormat(name string) (Parser, error) {
	switch name {
	case "jsonl":
		return jsonlParser{}, nil
	case "junit":
		return junitParser{}, nil
	case "gotest":
		return gotestParser{}, nil
	}
	return nil, &result.RejectError{Reason: result.ReasonUnknownFormat, Detail: "format " + name}
}
