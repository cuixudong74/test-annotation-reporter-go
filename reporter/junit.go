package reporter

import (
	"encoding/xml"
	"io"
	"strconv"
	"strings"
)

// junitTestcase 对应 JUnit XML 中的 <testcase> 元素。
type junitTestcase struct {
	ClassName string `xml:"classname,attr"`
	Name      string `xml:"name,attr"`
	File      string `xml:"file,attr"`
	Line      string `xml:"line,attr"`
	Failure   *struct {
		Message string `xml:"message,attr"`
		Text    string `xml:",chardata"`
	} `xml:"failure"`
	Error *struct {
		Message string `xml:"message,attr"`
		Text    string `xml:",chardata"`
	} `xml:"error"`
	Skipped *struct {
		Message string `xml:"message,attr"`
	} `xml:"skipped"`
}

// parseJUnitXML 用 xml.Decoder 流式逐个解析 <testcase>，
// 结构损坏时产出 Reject 记录并尝试继续。
func parseJUnitXML(shard string, r io.Reader, out chan<- Record) error {
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			// XML 结构损坏：无法可靠地继续扫描，明确拒绝剩余输入。
			out <- Record{Reject: &RejectError{Reason: RejectMalformed,
				Detail: "invalid XML stream: " + err.Error()}}
			return nil
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "testcase" {
			continue
		}
		var tc junitTestcase
		if err := dec.DecodeElement(&tc, &se); err != nil {
			out <- Record{Reject: &RejectError{Reason: RejectMalformed,
				Detail: "invalid testcase element: " + err.Error()}}
			continue
		}
		emitJUnitTestcase(shard, tc, out)
	}
}

func emitJUnitTestcase(shard string, tc junitTestcase, out chan<- Record) {
	id := tc.ClassName
	if id != "" && tc.Name != "" {
		id += "."
	}
	id += tc.Name
	if id == "" {
		out <- Record{Reject: &RejectError{Reason: RejectMissingID,
			Detail: "testcase without name/classname"}}
		return
	}
	res := &TestResult{ID: id, Name: id, Shard: shard, File: tc.File}
	if n, err := strconv.Atoi(tc.Line); err == nil {
		res.Line = n
	}
	switch {
	case tc.Failure != nil:
		res.Status = StatusFailed
		res.Message = firstNonEmpty(tc.Failure.Message, strings.TrimSpace(tc.Failure.Text))
	case tc.Error != nil:
		res.Status = StatusError
		res.Message = firstNonEmpty(tc.Error.Message, strings.TrimSpace(tc.Error.Text))
	case tc.Skipped != nil:
		res.Status = StatusSkipped
		res.Message = tc.Skipped.Message
	default:
		res.Status = StatusPassed
	}
	// 若未提供 file/line 属性，尝试从失败消息与详情文本中提取 "file.go:12:"。
	if res.File == "" {
		var texts []string
		if tc.Failure != nil {
			texts = append(texts, tc.Failure.Message, tc.Failure.Text)
		}
		if tc.Error != nil {
			texts = append(texts, tc.Error.Message, tc.Error.Text)
		}
		for _, text := range texts {
			found := false
			for _, ln := range strings.Split(text, "\n") {
				if m := goFileLineRe.FindStringSubmatch(ln); m != nil {
					res.File = m[1]
					res.Line, _ = strconv.Atoi(m[2])
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	out <- Record{Result: res}
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
