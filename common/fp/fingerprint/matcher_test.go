package fingerprint

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/fp/fingerprint/parsers"
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
	matchRes := matcher.Match(raw)
	println()
	_ = matchRes
}

func TestExpressionMatch(t *testing.T) {
	rules1 := LoadAllDefaultRules()
	_= rules1
	content, err := rule_resources.FS.ReadFile("exp_rule.txt")
	if err != nil {
		t.Fatal(err)
	}
	ruleInfos := funk.Map(strings.Split(string(content), "\n"), func(s string) [2]string {
		splits := strings.Split(s, "\x00")
		return [2]string{splits[1], splits[0]}
	})
	rules, err := parsers.ParseExpRule(ruleInfos.([][2]string))
	if err != nil {
		t.Fatal(err)
	}
	matcher := NewMatcher(rules...)
	info := matcher.Match([]byte(`HTTP/1.1 200 OK
Tag: --- VIDEO WEB SERVER ---


<!doctype html>
<html>
/AV732E/setup.exe
</html>
`))
	assert.Equal(t, info[0].Info, "AVTech-Video-Web-Server")
}
