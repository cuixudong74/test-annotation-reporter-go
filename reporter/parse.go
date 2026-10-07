package reporter

import (
	"bufio"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Format 标识输入结果格式。
type Format int

const (
	FormatGoTestJSON Format = iota // go test -json 输出
	FormatJUnitXML                 // JUnit XML 报告
)

// Record 是解析产出：要么是一条结果，要么是一条拒绝记录。
type Record struct {
	Result *TestResult
	Reject *RejectError
}

// ParseStream 从 r 流式解析 format 格式的记录，逐条送入 out。
// 损坏或非法的记录以 Reject 形式产出，不会中断后续解析。
// 调用方负责关闭 out。
func ParseStream(format Format, shard string, r io.Reader, out chan<- Record) error {
	switch format {
	case FormatGoTestJSON:
		return parseGoTestJSON(shard, r, out)
	case FormatJUnitXML:
		return parseJUnitXML(shard, r, out)
	}
	return &RejectError{Reason: RejectUnknownFormat, Detail: "unsupported format id"}
}

// goTestEvent 是 go test -json 的一行事件。
type goTestEvent struct {
	Action  string
	Package string
	Test    string
	Output  string
}

// 匹配输出行中的源码位置，例如 "    foo_test.go:12: boom"。
var goFileLineRe = regexp.MustCompile(`^\s*([^\s:()]+\.go):(\d+):\s*(.*)$`)

type goTestState struct {
	output  []string
	file    string
	line    int
	message string
}

func parseGoTestJSON(shard string, r io.Reader, out chan<- Record) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	states := map[string]*goTestState{}
	key := func(pkg, test string) string { return pkg + "\x00" + test }

	flush := func(pkg, test string, st Status) {
		s := states[key(pkg, test)]
		id := pkg
		if test != "" {
			id = pkg + "." + test
		}
		res := &TestResult{ID: id, Name: id, Status: st, Shard: shard}
		if s != nil {
			res.File = s.file
			res.Line = s.line
			res.Message = strings.Join(s.output, "\n")
		}
		out <- Record{Result: res}
		delete(states, key(pkg, test))
	}

	for sc.Scan() {
		line := sc.Bytes()
		var ev goTestEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			out <- Record{Reject: &RejectError{Reason: RejectMalformed,
				Detail: "invalid JSON line: " + truncate(string(line), 80)}}
			continue
		}
		k := key(ev.Package, ev.Test)
		switch ev.Action {
		case "output":
			s := states[k]
			if s == nil {
				s = &goTestState{}
				states[k] = s
			}
			text := strings.TrimRight(ev.Output, "\n")
			s.output = append(s.output, text)
			if s.file == "" {
				if m := goFileLineRe.FindStringSubmatch(text); m != nil {
					s.file = m[1]
					s.line, _ = strconv.Atoi(m[2])
					if s.message == "" {
						s.message = m[3]
					}
				}
			}
		case "pass":
			if ev.Test != "" {
				flush(ev.Package, ev.Test, StatusPassed)
			}
		case "fail":
			if ev.Test != "" {
				flush(ev.Package, ev.Test, StatusFailed)
			} else if ev.Package != "" {
				// 包级失败（如编译错误）记为 error。
				flush(ev.Package, "", StatusError)
			}
		case "skip":
			if ev.Test != "" {
				flush(ev.Package, ev.Test, StatusSkipped)
			}
		case "run", "pause", "cont", "bench", "start":
			// 已知但不产生结果的事件，忽略。
		default:
			out <- Record{Reject: &RejectError{Reason: RejectUnknownFormat,
				Detail: "unknown action: " + ev.Action}}
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
