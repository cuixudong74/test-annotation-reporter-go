// Package result defines the canonical test result model shared by
// ingestion, merging and annotation rendering.
package result

// Status is the outcome of a single test case execution.
type Status int

const (
	StatusPassed Status = iota
	StatusFailed
	StatusSkipped
	StatusError
)

func (s Status) String() string {
	switch s {
	case StatusPassed:
		return "passed"
	case StatusFailed:
		return "failed"
	case StatusSkipped:
		return "skipped"
	case StatusError:
		return "error"
	}
	return "unknown"
}

// TestResult is one execution record of one test case.
type TestResult struct {
	Suite   string // test suite / package name
	Name    string // test case name
	Status  Status
	File    string // source file, may be empty
	Line    int    // 1-based source line, 0 when unknown
	Message string // failure/error detail
	Shard   string // originating shard / executor id
	Seq     uint64 // monotonic sequence within the shard
}

// Key identifies a test case for deduplication across shards.
func (r TestResult) Key() string { return r.Suite + "\x00" + r.Name }

// RejectReason classifies why an input record was refused.
type RejectReason int

const (
	ReasonMalformed     RejectReason = iota // record cannot be parsed
	ReasonUnknownFormat                     // record type/format not supported
	ReasonMissingField                      // required field absent
	ReasonDuplicate                         // already ingested (resumed stream)
)

func (r RejectReason) String() string {
	switch r {
	case ReasonMalformed:
		return "malformed"
	case ReasonUnknownFormat:
		return "unknown-format"
	case ReasonMissingField:
		return "missing-field"
	case ReasonDuplicate:
		return "duplicate"
	}
	return "unknown"
}

// RejectError describes a refused input record.
type RejectError struct {
	Reason RejectReason
	Detail string
}

func (e *RejectError) Error() string { return e.Reason.String() + ": " + e.Detail }
