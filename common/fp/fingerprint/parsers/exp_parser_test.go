package parsers

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func _TestCompilerSpecialSyntax(t *testing.T) {
	rules, err := ParseExpRule([][2]string{{`header=""MiniCMS""`, "a"}})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, `"MiniCMS"`, rules[0].MatchParam.Params[1])
	rules, err = ParseExpRule([][2]string{{` body="VALUE="Copyright (C) 2000, Cobalt Networks"`, "a"}})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, `VALUE="Copyright (C) 2000, Cobalt Networks`, rules[0].MatchParam.Params[1])
	rules, err = ParseExpRule([][2]string{{`(body="Everything.gif"||body="everything.png") && title=="Everything"`, "a"}})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, `and`, rules[0].MatchParam.Condition)

	rules, err = ParseExpRule([][2]string{{`body="xheditor_lang/zh-cn.js"||body="class="xheditor"||body=".xheditor("`, "a"}})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, `or`, rules[0].MatchParam.Condition)

	rules, err = ParseExpRule([][2]string{{`server="TornadoServer"&&Celery`, "a"}})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, `and`, rules[0].MatchParam.Condition)
	assert.Equal(t, `raw`, rules[0].MatchParam.SubRules[1].MatchParam.Params[0])
}
func TestCompiler(t *testing.T) {
	rules, err := ParseExpRule([][2]string{{`header="\"MiniCMS\""`, "a"}})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, `"MiniCMS"`, rules[0].MatchParam.Params[1])

	rules, err = ParseExpRule([][2]string{{"header=\"MiniCMS\" || title=\"MiniCMS\"", "a"}})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "complex", rules[0].Method)
	assert.Equal(t, "or", rules[0].MatchParam.Condition)
	assert.Equal(t, 2, len(rules[0].MatchParam.SubRules))
}
