package annotate

import (
	"encoding/json"
	"hash/fnv"
	"strconv"
)

// gitlabRenderer emits GitLab Code Quality report entries, one JSON object
// per line. JSON encoding itself provides the escaping, so no special
// character can break the format or inject directives.
type gitlabRenderer struct{}

type glLocation struct {
	Path  string `json:"path"`
	Lines struct {
		Begin int `json:"begin"`
	} `json:"lines"`
}

type glIssue struct {
	Type        string     `json:"type"`
	CheckName   string     `json:"check_name"`
	Description string     `json:"description"`
	Categories  []string   `json:"categories"`
	Severity    string     `json:"severity"`
	Fingerprint string     `json:"fingerprint"`
	Location    glLocation `json:"location"`
}

func (gitlabRenderer) Render(a Annotation) string {
	var issue glIssue
	issue.Type = "issue"
	issue.CheckName = "test-annotation-reporter"
	issue.Description = a.Message
	issue.Categories = []string{"Bug Risk"}
	if a.Level == LevelError {
		issue.Severity = "major"
	} else {
		issue.Severity = "minor"
	}
	issue.Location.Path = a.File
	issue.Location.Lines.Begin = a.Line
	// Deterministic fingerprint so re-runs produce byte-identical output.
	h := fnv.New64a()
	h.Write([]byte(a.File))
	h.Write([]byte{0})
	h.Write([]byte(strconv.Itoa(a.Line)))
	h.Write([]byte{0})
	h.Write([]byte(a.Message))
	issue.Fingerprint = strconv.FormatUint(h.Sum64(), 16)
	out, _ := json.Marshal(issue)
	return string(out)
}
