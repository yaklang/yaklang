package parsers

import (
	"github.com/yaklang/yaklang/embed"
	"testing"
)

func TestMatcher(t *testing.T) {
	content, err := embed.Asset("data/fingerprint-rules.yml.gz")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := ParseYamlRule(string(content))
	if err != nil {
		t.Fatal(err)
	}
	_ = rules
}

func TestName(t *testing.T) {

}
