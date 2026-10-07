package annotate

import (
	"strconv"
	"strings"
)

// githubRenderer emits GitHub Actions workflow commands:
//
//	::error file={file},line={line}::{message}
//
// Escaping follows the toolkit rules so special characters can neither
// truncate the annotation nor inject extra commands.
type githubRenderer struct{}

// ghEscapeData escapes message payloads: %, \r, \n.
func ghEscapeData(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

// ghEscapeProp escapes property values: data escapes plus ':' and ','.
func ghEscapeProp(s string) string {
	s = ghEscapeData(s)
	s = strings.ReplaceAll(s, ":", "%3A")
	s = strings.ReplaceAll(s, ",", "%2C")
	return s
}

func (githubRenderer) Render(a Annotation) string {
	var b strings.Builder
	b.WriteString("::")
	if a.Level == LevelError {
		b.WriteString("error")
	} else {
		b.WriteString("warning")
	}
	var props []string
	if a.File != "" {
		props = append(props, "file="+ghEscapeProp(a.File))
	}
	if a.Line > 0 {
		props = append(props, "line="+strconv.Itoa(a.Line))
	}
	if len(props) > 0 {
		b.WriteByte(' ')
		b.WriteString(strings.Join(props, ","))
	}
	b.WriteString("::")
	b.WriteString(ghEscapeData(a.Message))
	return b.String()
}
