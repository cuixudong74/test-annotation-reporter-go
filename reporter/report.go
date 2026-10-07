package reporter

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Report 是一份归并后的报告快照。
type Report struct {
	Results []TestResult
	Counts  map[Status]int
}

// Failed 返回需要产出注解的结果（failed 与 error），保持报告内排序。
func (r Report) Failed() []TestResult {
	var out []TestResult
	for _, res := range r.Results {
		if res.Status == StatusFailed || res.Status == StatusError {
			out = append(out, res)
		}
	}
	return out
}

// WriteText 以确定性字节流输出报告：结果按 ID 排序、字段顺序固定、
// 不含时间戳，固定输入下重复运行逐字节一致。
func (r Report) WriteText(w io.Writer) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "total=%d passed=%d failed=%d skipped=%d error=%d\n",
		len(r.Results),
		r.Counts[StatusPassed], r.Counts[StatusFailed],
		r.Counts[StatusSkipped], r.Counts[StatusError])
	for _, res := range r.Results {
		loc := "-"
		if res.File != "" {
			loc = fmt.Sprintf("%s:%d", res.File, res.Line)
		}
		msg := strings.ReplaceAll(res.Message, "\n", `\n`)
		fmt.Fprintf(bw, "%s\t%s\t%s\t%s\n", strings.ToUpper(res.Status.String()), res.ID, loc, msg)
	}
	return bw.Flush()
}
