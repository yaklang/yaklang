package sfaudit

import (
	"regexp"
	"strings"
	"testing"
)

// ruleIDPattern pins the rule ID convention: lowercase dot-separated
// segments, snake_case inside a segment, and the first segment marks the
// scope — a language (java/py/go/php/node) or the language-agnostic
// secret namespace.
var ruleIDPattern = regexp.MustCompile(`^(java|py|go|php|node|secret)(\.[a-z0-9_]+)+$`)

func TestRuleNamingConvention(t *testing.T) {
	entries, err := ruleFS.ReadDir("rules")
	if err != nil {
		t.Fatalf("read embedded rules: %v", err)
	}
	for _, ent := range entries {
		name := ent.Name()
		if !strings.HasSuffix(name, ".sf") {
			continue
		}
		content, err := ruleFS.ReadFile("rules/" + name)
		if err != nil {
			t.Fatalf("read rule %s: %v", name, err)
		}
		id, ok := extractRuleID(string(content))
		if !ok {
			t.Errorf("%s: missing rule_id in desc header", name)
			continue
		}
		if !ruleIDPattern.MatchString(id) {
			t.Errorf("%s: rule_id %q violates the naming convention (lowercase dot-separated snake_case with a language or secret prefix)", name, id)
		}
		// The file name must mirror the rule ID with '.' and '_' as '-'.
		want := strings.NewReplacer(".", "-", "_", "-").Replace(id) + ".sf"
		if name != want {
			t.Errorf("%s: file name must mirror rule_id %q (expected %s)", name, id, want)
		}
	}
}
