package subprocess

import "strings"

// BuildChildEnvironment strips the named variables from parent
// (case-insensitive) and appends extra entries.
func BuildChildEnvironment(parent []string, exclude []string, extra []string) []string {
	excludeSet := make(map[string]struct{}, len(exclude))
	for _, name := range exclude {
		excludeSet[strings.ToUpper(name)] = struct{}{}
	}
	result := make([]string, 0, len(parent)+len(extra))
	for _, entry := range parent {
		key, _, _ := strings.Cut(entry, "=")
		if _, ok := excludeSet[strings.ToUpper(key)]; ok {
			continue
		}
		result = append(result, entry)
	}
	return append(result, extra...)
}
