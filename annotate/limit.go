package annotate

import (
	"sort"
	"strconv"
)

// Limit caps the number of annotations. Entries identical in
// (file, line, message) are collapsed first so the same problem at the
// same location is never reported twice. Remaining entries are ordered by
// priority (errors before warnings, then file, line, message) so the cap
// always drops the least important ones deterministically. When entries
// are omitted, a trailing summary annotation reports how many.
func Limit(anns []Annotation, max int) []Annotation {
	seen := make(map[string]struct{}, len(anns))
	deduped := make([]Annotation, 0, len(anns))
	for _, a := range anns {
		key := a.File + "\x00" + strconv.Itoa(a.Line) + "\x00" + a.Message
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, a)
	}
	sort.SliceStable(deduped, func(i, j int) bool {
		a, b := deduped[i], deduped[j]
		if a.Level != b.Level {
			return a.Level > b.Level // errors (higher) first
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Message < b.Message
	})
	if max < 0 {
		max = 0
	}
	if len(deduped) <= max {
		return deduped
	}
	omitted := len(deduped) - max
	kept := make([]Annotation, 0, max+1)
	kept = append(kept, deduped[:max]...)
	kept = append(kept, Annotation{
		Level:   LevelWarning,
		Message: strconv.Itoa(omitted) + " further annotation(s) omitted (cap reached)",
	})
	return kept
}
