package parsers

import "testing"

func TestMatcher(t *testing.T) {
	rules, err := ParseYamlRule("")
	if err != nil {
		t.Fatal(err)
	}
	_ = rules
}
