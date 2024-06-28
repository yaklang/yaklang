package fingerprint

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/fp/fingerprint/parsers"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule_resources"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/embed"
	"strings"
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
	matchRes := matcher.Match(context.Background(), raw)
	_ = matchRes
}

func TestExpressionMatch(t *testing.T) {
	rules1 := LoadAllDefaultRules()
	_ = rules1
	content, err := rule_resources.FS.ReadFile("exp_rule.txt")
	if err != nil {
		t.Fatal(err)
	}
	ruleInfos := funk.Map(strings.Split(string(content), "\n"), func(s string) *rule.GeneralRule {
		splits := strings.Split(s, "\x00")
		return &rule.GeneralRule{MatchExpression: splits[1], CPE: &rule.CPE{Product: splits[0]}}
	})
	rules, _ := parsers.ParseExpRule(ruleInfos.([]*rule.GeneralRule)...)
	matcher := NewMatcher(rules...)
	info := matcher.Match(context.Background(), []byte(`HTTP/1.1 200 OK
Tag: --- VIDEO WEB SERVER ---


<!doctype html>
<html>
/AV732E/setup.exe
</html>
`))
	assert.Equal(t, info[0].Product, "AVTech-Video-Web-Server")
}
