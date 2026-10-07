package ingest

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/cuixudong74/test-annotation-reporter-go/result"
)

// junitParser parses JUnit XML reports. It decodes incrementally so results
// before a corruption point are preserved; the corrupt tail is rejected
// with a malformed reason.
type junitParser struct{}

type junitTestcase struct {
	XMLName   xml.Name      `xml:"testcase"`
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	File      string        `xml:"file,attr"`
	Line      string        `xml:"line,attr"`
	Failure   *junitPayload `xml:"failure"`
	Error     *junitPayload `xml:"error"`
	Skipped   *junitPayload `xml:"skipped"`
}

type junitPayload struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

func (p *junitPayload) text() string {
	if p == nil {
		return ""
	}
	if p.Message != "" {
		return p.Message
	}
	return strings.TrimSpace(p.Body)
}

func (junitParser) Parse(r io.Reader, shard string, emit func(result.TestResult), onReject func(*result.RejectError)) Stats {
	var st Stats
	dec := xml.NewDecoder(r)
	var seq uint64
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return st
		}
		if err != nil {
			st.Rejected++
			onReject(&result.RejectError{Reason: result.ReasonMalformed,
				Detail: fmt.Sprintf("xml: %v", err)})
			return st
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "testcase" {
			continue
		}
		var tc junitTestcase
		if err := dec.DecodeElement(&tc, &se); err != nil {
			st.Rejected++
			onReject(&result.RejectError{Reason: result.ReasonMalformed,
				Detail: fmt.Sprintf("testcase: %v", err)})
			continue
		}
		if tc.Name == "" {
			st.Rejected++
			onReject(&result.RejectError{Reason: result.ReasonMissingField,
				Detail: "testcase without name"})
			continue
		}
		tr := result.TestResult{
			Suite: tc.Classname,
			Name:  tc.Name,
			File:  tc.File,
			Shard: shard,
			Seq:   seq,
		}
		seq++
		if n, err := strconv.Atoi(tc.Line); err == nil && n > 0 {
			tr.Line = n
		}
		switch {
		case tc.Failure != nil:
			tr.Status = result.StatusFailed
			tr.Message = tc.Failure.text()
		case tc.Error != nil:
			tr.Status = result.StatusError
			tr.Message = tc.Error.text()
		case tc.Skipped != nil:
			tr.Status = result.StatusSkipped
			tr.Message = tc.Skipped.text()
		default:
			tr.Status = result.StatusPassed
		}
		emit(tr)
		st.Parsed++
	}
}
