package fingerprint

import (
	"github.com/yaklang/yaklang/common/fp/fingerprint/parsers"
	"github.com/yaklang/yaklang/common/log"
	"testing"
)

func TestMatch(t *testing.T) {
	rules, err := parsers.ParseYamlRule("")
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("")
	matcher := NewMatcher()
	matcher.ErrorHandle = func(err error) {
		log.Error(err)
	}
	matchRes := matcher.Match(raw, rules)
	println()
	_ = matchRes
}
