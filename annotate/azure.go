package annotate

import (
	"strconv"
	"strings"
)

// azureRenderer emits Azure DevOps logging commands:
//
//	##vso[task.logissue type=error;sourcepath={file};linenumber={line}]{message}
type azureRenderer struct{}

// azEscapeData escapes message payloads: %, \r, \n.
func azEscapeData(s string) string {
	s = strings.ReplaceAll(s, "%", "%AZP25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

// azEscapeProp escapes property values: data escapes plus ';' and ']'.
func azEscapeProp(s string) string {
	s = azEscapeData(s)
	s = strings.ReplaceAll(s, ";", "%3B")
	s = strings.ReplaceAll(s, "]", "%5D")
	return s
}

func (azureRenderer) Render(a Annotation) string {
	var b strings.Builder
	b.WriteString("##vso[task.logissue type=")
	if a.Level == LevelError {
		b.WriteString("error")
	} else {
		b.WriteString("warning")
	}
	if a.File != "" {
		b.WriteString(";sourcepath=")
		b.WriteString(azEscapeProp(a.File))
	}
	if a.Line > 0 {
		b.WriteString(";linenumber=")
		b.WriteString(strconv.Itoa(a.Line))
	}
	b.WriteByte(']')
	b.WriteString(azEscapeData(a.Message))
	return b.String()
}
