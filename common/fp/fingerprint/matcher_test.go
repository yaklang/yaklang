package fingerprint

import (
	"github.com/yaklang/yaklang/common/fp/fingerprint/parsers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/embed"
	"testing"
)

func TestMatch(t *testing.T) {
	content, err := embed.Asset("data/fingerprint-rules.yml.gz")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := parsers.ParseYamlRule(string(content))
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("")
	matcher := NewMatcher(rules...)
	matcher.ErrorHandle = func(err error) {
		log.Error(err)
	}
	matchRes := matcher.Match(raw)
	println()
	_ = matchRes
}
