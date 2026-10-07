// Package annotate renders failed test results as CI platform annotations
// with platform-correct escaping, caps and aggregation.
package annotate

import (
	"github.com/cuixudong74/test-annotation-reporter-go/result"
)

// Level of an annotation.
type Level int

const (
	LevelWarning Level = iota
	LevelError
)

// Annotation is one platform-neutral annotation.
type Annotation struct {
	Level   Level
	File    string
	Line    int
	Message string
}

// Renderer converts annotations into a platform-specific text format.
type Renderer interface {
	Render(a Annotation) string
}

// ForPlatform returns a renderer for github, gitlab or azure.
func ForPlatform(name string) (Renderer, error) {
	switch name {
	case "github":
		return githubRenderer{}, nil
	case "gitlab":
		return gitlabRenderer{}, nil
	case "azure":
		return azureRenderer{}, nil
	}
	return nil, errUnknownPlatform(name)
}

type errUnknownPlatform string

func (e errUnknownPlatform) Error() string { return "unknown platform: " + string(e) }

// FromResults converts failing results (failed or error) into annotations,
// in input order. Passed and skipped results produce no annotations.
func FromResults(results []result.TestResult) []Annotation {
	var anns []Annotation
	for _, r := range results {
		if r.Status != result.StatusFailed && r.Status != result.StatusError {
			continue
		}
		msg := r.Suite + "." + r.Name
		if r.Message != "" {
			msg += ": " + r.Message
		}
		anns = append(anns, Annotation{
			Level:   LevelError,
			File:    r.File,
			Line:    r.Line,
			Message: msg,
		})
	}
	return anns
}
