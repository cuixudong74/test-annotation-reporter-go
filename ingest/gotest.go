package ingest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/cuixudong74/test-annotation-reporter-go/result"
)

// gotestParser parses `go test -json` event streams. Output lines are
// buffered per test only until its terminal event, so memory stays bounded
// by the number of concurrently running tests.
type gotestParser struct{}

type goEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

// locRe matches go test output lines like "    x_test.go:42: boom".
var locRe = regexp.MustCompile(`^\s*(\S+\.go):(\d+):\s?(.*)$`)

func (gotestParser) Parse(r io.Reader, shard string, emit func(result.TestResult), onReject func(*result.RejectError)) Stats {
	var st Stats
	type pending struct {
		file, msg string
		line      int
		out       strings.Builder
	}
	open := map[string]*pending{} // package\x00test -> accumulated output
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	var seq, lineNo uint64
	for sc.Scan() {
		line := sc.Bytes()
		lineNo++
		if len(line) == 0 {
			continue
		}
		var ev goEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			st.Rejected++
			onReject(&result.RejectError{Reason: result.ReasonMalformed,
				Detail: fmt.Sprintf("line %d: %v", lineNo, err)})
			continue
		}
		key := ev.Package + "\x00" + ev.Test
		switch ev.Action {
		case "output":
			p := open[key]
			if p == nil {
				p = &pending{}
				open[key] = p
			}
			p.out.WriteString(ev.Output)
			if m := locRe.FindStringSubmatch(strings.TrimRight(ev.Output, "\n")); m != nil && p.file == "" {
				if n, err := strconv.Atoi(m[2]); err == nil {
					p.file, p.line, p.msg = m[1], n, m[3]
				}
			}
		case "pass", "fail", "skip":
			if ev.Test == "" {
				continue // package-level summary, not a test case
			}
			p := open[key]
			delete(open, key)
			tr := result.TestResult{
				Suite: ev.Package, Name: ev.Test, Shard: shard, Seq: seq,
			}
			seq++
			switch ev.Action {
			case "pass":
				tr.Status = result.StatusPassed
			case "skip":
				tr.Status = result.StatusSkipped
			case "fail":
				tr.Status = result.StatusFailed
			}
			if p != nil {
				tr.File, tr.Line = p.file, p.line
				if tr.Status != result.StatusPassed {
					tr.Message = p.msg
					if tr.Message == "" {
						tr.Message = strings.TrimSpace(p.out.String())
					}
				}
			}
			emit(tr)
			st.Parsed++
		case "run":
			if ev.Test != "" {
				if _, ok := open[key]; !ok {
					open[key] = &pending{}
				}
			}
		case "pause", "cont", "bench", "start":
			// known non-terminal events, nothing to do
		default:
			st.Rejected++
			onReject(&result.RejectError{Reason: result.ReasonUnknownFormat,
				Detail: fmt.Sprintf("line %d: unknown action %q", lineNo, ev.Action)})
		}
	}
	if err := sc.Err(); err != nil {
		st.Rejected++
		onReject(&result.RejectError{Reason: result.ReasonMalformed, Detail: err.Error()})
	}
	// Stream interrupted before terminal events: mark unfinished tests as
	// errors instead of silently losing them.
	for key, p := range open {
		parts := strings.SplitN(key, "\x00", 2)
		if parts[1] == "" {
			continue
		}
		emit(result.TestResult{
			Suite: parts[0], Name: parts[1], Status: result.StatusError,
			File: p.file, Line: p.line,
			Message: "stream interrupted before test finished",
			Shard:   shard, Seq: seq,
		})
		seq++
		st.Parsed++
	}
	return st
}
