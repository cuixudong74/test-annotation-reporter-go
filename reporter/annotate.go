package reporter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Platform 标识目标流水线平台。
type Platform int

const (
	PlatformGitHub Platform = iota // GitHub Actions workflow commands
	PlatformAzure                  // Azure Pipelines logging commands
	PlatformGitLab                 // GitLab（JSON Lines，供 codequality/自定义解析）
)

// annotation 是一条待输出的平台无关注解。
type annotation struct {
	Severity string // "error" 或 "warning"
	File     string
	Line     int
	Message  string
}

// key 用于按"同一位置同一原因"去重。
func (a annotation) key() string {
	return fmt.Sprintf("%s\x00%d\x00%s", a.File, a.Line, a.Message)
}

// Annotator 把失败用例转换为平台注解。
type Annotator struct {
	Platform Platform
	Max      int // 注解数量上限，0 表示不限制
}

// collect 从报告中提取注解：按 (file,line,message) 去重，
// 再按既定优先级（error 优先，其后 file/line/message 字典序）排序，保证确定性。
func collect(rep Report) []annotation {
	seen := map[string]bool{}
	var out []annotation
	for _, res := range rep.Failed() {
		sev := "error"
		msg := res.Message
		if msg == "" {
			msg = res.Name + " " + res.Status.String()
		}
		a := annotation{Severity: sev, File: res.File, Line: res.Line, Message: msg}
		k := a.key()
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity > out[j].Severity // "error" > "warning"
		}
		return out[i].key() < out[j].key()
	})
	return out
}

// WriteAnnotations 将报告中的失败/错误用例写成平台注解。
// 超出 Max 时按优先级保留，并追加一条聚合注解说明被省略的数量。
func (a Annotator) WriteAnnotations(rep Report, w io.Writer) error {
	anns := collect(rep)
	omitted := 0
	if a.Max > 0 && len(anns) > a.Max {
		omitted = len(anns) - a.Max
		anns = anns[:a.Max]
	}
	bw := bufio.NewWriter(w)
	for _, an := range anns {
		if err := a.writeOne(bw, an); err != nil {
			return err
		}
	}
	if omitted > 0 {
		summary := annotation{
			Severity: "warning",
			Message:  fmt.Sprintf("%d annotation(s) omitted due to limit", omitted),
		}
		if err := a.writeOne(bw, summary); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func (a Annotator) writeOne(w io.Writer, an annotation) error {
	switch a.Platform {
	case PlatformGitHub:
		_, err := fmt.Fprintf(w, "::%s file=%s,line=%d::%s\n",
			an.Severity, escapeGitHubProp(an.File), an.Line, escapeGitHubData(an.Message))
		return err
	case PlatformAzure:
		_, err := fmt.Fprintf(w, "##vso[task.logissue type=%s;sourcepath=%s;linenumber=%d;]%s\n",
			an.Severity, escapeAzure(an.File), an.Line, escapeAzure(an.Message))
		return err
	case PlatformGitLab:
		b, err := json.Marshal(map[string]any{
			"type":     "test-annotation",
			"severity": an.Severity,
			"file":     an.File,
			"line":     an.Line,
			"message":  an.Message,
		})
		if err != nil {
			return err
		}
		_, err = w.Write(append(b, '\n'))
		return err
	}
	return fmt.Errorf("unknown platform: %d", a.Platform)
}

// escapeGitHubData 转义 GitHub 注解消息体：%、\r、\n。
func escapeGitHubData(s string) string {
	r := strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
	)
	return r.Replace(s)
}

// escapeGitHubProp 转义 GitHub 注解属性值：额外转义 ":" 和 ","，
// 防止文件路径截断属性列表或注入额外指令。
func escapeGitHubProp(s string) string {
	r := strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
		":", "%3A",
		",", "%2C",
	)
	return r.Replace(s)
}

// escapeAzure 转义 Azure logging command 中的特殊字符。
func escapeAzure(s string) string {
	r := strings.NewReplacer(
		"%", "%AZP25",
		"\r", "%0D",
		"\n", "%0A",
		"]", "%5D",
		";", "%3B",
	)
	return r.Replace(s)
}

// IncrementalAnnotator 支持流式增量输出：失败到达时立即产出注解，
// 已产出的 (file,line,message) 组合不会重复输出，输入中断后
// 继续摄入也不会重复注解。
type IncrementalAnnotator struct {
	ann     Annotator
	emitted map[string]bool
}

func NewIncrementalAnnotator(a Annotator) *IncrementalAnnotator {
	return &IncrementalAnnotator{ann: a, emitted: map[string]bool{}}
}

// Add 处理一条结果；若为失败/错误且未产出过，则立即写出注解。
// 返回是否实际产出了注解。Max 上限同样生效。
func (ia *IncrementalAnnotator) Add(res TestResult, w io.Writer) (bool, error) {
	if res.Status != StatusFailed && res.Status != StatusError {
		return false, nil
	}
	msg := res.Message
	if msg == "" {
		msg = res.Name + " " + res.Status.String()
	}
	a := annotation{Severity: "error", File: res.File, Line: res.Line, Message: msg}
	k := a.key()
	if ia.emitted[k] {
		return false, nil
	}
	if ia.ann.Max > 0 && len(ia.emitted) >= ia.ann.Max {
		return false, nil
	}
	if err := ia.ann.writeOne(w, a); err != nil {
		return false, err
	}
	ia.emitted[k] = true
	return true, nil
}
