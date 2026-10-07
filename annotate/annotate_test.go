package annotate

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cuixudong74/test-annotation-reporter-go/result"
)

var tricky = Annotation{
	Level:   LevelError,
	File:    "src/weird:dir/a,b.go",
	Line:    3,
	Message: "boom\nnext line\r\n100% ::error file=x::injected ##vso[task.cancel]",
}

func TestGitHubEscaping(t *testing.T) {
	out := githubRenderer{}.Render(tricky)
	if !strings.HasPrefix(out, "::error file=") {
		t.Fatalf("bad prefix: %q", out)
	}
	// Raw newlines would let the message inject a second workflow
	// command; they must be escaped. Unescaped ',' and ':' in the file
	// property would corrupt the property list.
	for _, bad := range []string{"\n", "\r", "a,b.go", "weird:dir"} {
		if strings.Contains(out, bad) {
			t.Fatalf("unescaped %q in %q", bad, out)
		}
	}
	for _, want := range []string{"a%2Cb.go", "weird%3Adir", "%0A", "100%25"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestAzureEscaping(t *testing.T) {
	out := azureRenderer{}.Render(tricky)
	if !strings.HasPrefix(out, "##vso[task.logissue type=error;") {
		t.Fatalf("bad prefix: %q", out)
	}
	// Raw newlines would let the message inject a second logging command.
	for _, bad := range []string{"\n", "\r"} {
		if strings.Contains(out, bad) {
			t.Fatalf("unescaped %q in %q", bad, out)
		}
	}
	// The property block ends at the first ']'; inside it no raw ';' or
	// ']' from user data may appear.
	close := strings.Index(out, "]")
	props := out[:close]
	if strings.Count(props, ";") != 2 {
		t.Fatalf("property block corrupted: %q", props)
	}
	for _, want := range []string{"%0A", "%AZP25", "sourcepath=src/weird:dir/a,b.go"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestGitLabEscaping(t *testing.T) {
	out := gitlabRenderer{}.Render(tricky)
	if strings.Contains(out, "\n") || strings.Contains(out, "\r") {
		t.Fatalf("raw newline in %q", out)
	}
	if !strings.Contains(out, `"boom\nnext line\r\n100%`) {
		t.Fatalf("message not JSON-escaped: %q", out)
	}
	// Deterministic fingerprint across renders.
	if out != (gitlabRenderer{}).Render(tricky) {
		t.Fatal("gitlab render not deterministic")
	}
}

func TestFromResultsOnlyFailures(t *testing.T) {
	anns := FromResults([]result.TestResult{
		{Suite: "s", Name: "A", Status: result.StatusPassed},
		{Suite: "s", Name: "B", Status: result.StatusFailed, File: "b.go", Line: 1, Message: "x"},
		{Suite: "s", Name: "C", Status: result.StatusSkipped},
		{Suite: "s", Name: "D", Status: result.StatusError, File: "d.go", Line: 2},
	})
	if len(anns) != 2 {
		t.Fatalf("got %d annotations", len(anns))
	}
	if anns[0].Message != "s.B: x" || anns[1].Message != "s.D" {
		t.Fatalf("bad messages: %v", anns)
	}
}

func TestLimitDeduplicatesAndCaps(t *testing.T) {
	var anns []Annotation
	// Same position + same reason repeated: must collapse to one.
	for i := 0; i < 5; i++ {
		anns = append(anns, Annotation{Level: LevelError, File: "a.go", Line: 1, Message: "same"})
	}
	for i := 0; i < 10; i++ {
		anns = append(anns, Annotation{Level: LevelError, File: "b.go", Line: i, Message: "distinct"})
	}
	out := Limit(anns, 4)
	if len(out) != 5 { // 4 kept + 1 summary
		t.Fatalf("len=%d", len(out))
	}
	summary := out[len(out)-1]
	if summary.Level != LevelWarning || !strings.Contains(summary.Message, "7 further annotation(s) omitted") {
		t.Fatalf("bad summary: %+v", summary)
	}
	// No duplicate (file,line,message) in kept entries.
	seen := map[string]bool{}
	for _, a := range out[:4] {
		k := fmt.Sprintf("%s:%d:%s", a.File, a.Line, a.Message)
		if seen[k] {
			t.Fatalf("duplicate kept: %s", k)
		}
		seen[k] = true
	}
}

func TestLimitDeterministic(t *testing.T) {
	var anns []Annotation
	for i := 9; i >= 0; i-- {
		anns = append(anns, Annotation{Level: LevelError, File: fmt.Sprintf("f%d.go", i), Line: i, Message: "m"})
	}
	a := Limit(anns, 3)
	b := Limit(anns, 3)
	if fmt.Sprintf("%v", a) != fmt.Sprintf("%v", b) {
		t.Fatal("Limit not deterministic")
	}
	if a[0].File != "f0.go" {
		t.Fatalf("priority order wrong: %v", a)
	}
}
