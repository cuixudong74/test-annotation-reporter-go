package ingest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cuixudong74/test-annotation-reporter-go/result"
)

// jsonlParser parses the native line-delimited JSON format, one record per
// line:
//
//	{"suite":"pkg","name":"TestX","status":"failed","file":"pkg/x.go",
//	 "line":42,"message":"boom","seq":3}
//
// seq is optional; when absent the line number is used so resume-dedup in
// the merger still works.
type jsonlParser struct{}

type jsonlRecord struct {
	Suite   string `json:"suite"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
	Seq     *int64 `json:"seq"`
}

func parseStatus(s string) (result.Status, bool) {
	switch s {
	case "passed", "pass":
		return result.StatusPassed, true
	case "failed", "fail":
		return result.StatusFailed, true
	case "skipped", "skip":
		return result.StatusSkipped, true
	case "error":
		return result.StatusError, true
	}
	return 0, false
}

func (jsonlParser) Parse(r io.Reader, shard string, emit func(result.TestResult), onReject func(*result.RejectError)) Stats {
	var st Stats
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	var lineNo uint64
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec jsonlRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			st.Rejected++
			onReject(&result.RejectError{Reason: result.ReasonMalformed,
				Detail: fmt.Sprintf("line %d: %v", lineNo, err)})
			lineNo++
			continue
		}
		if rec.Name == "" {
			st.Rejected++
			onReject(&result.RejectError{Reason: result.ReasonMissingField,
				Detail: fmt.Sprintf("line %d: missing name", lineNo)})
			lineNo++
			continue
		}
		status, ok := parseStatus(rec.Status)
		if !ok {
			st.Rejected++
			onReject(&result.RejectError{Reason: result.ReasonUnknownFormat,
				Detail: fmt.Sprintf("line %d: unknown status %q", lineNo, rec.Status)})
			lineNo++
			continue
		}
		seq := lineNo
		if rec.Seq != nil && *rec.Seq >= 0 {
			seq = uint64(*rec.Seq)
		}
		emit(result.TestResult{
			Suite: rec.Suite, Name: rec.Name, Status: status,
			File: rec.File, Line: rec.Line, Message: rec.Message,
			Shard: shard, Seq: seq,
		})
		st.Parsed++
		lineNo++
	}
	if err := sc.Err(); err != nil {
		st.Rejected++
		onReject(&result.RejectError{Reason: result.ReasonMalformed, Detail: err.Error()})
	}
	return st
}
