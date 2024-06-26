package parsers

import (
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule_resources"
	"github.com/yaklang/yaklang/common/go-funk"
	"os"
	"strings"
	"testing"
)

func TestCompilerSpecialSyntax(t *testing.T) {
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

func TestCompiler1(t *testing.T) {
	content, err := rule_resources.FS.ReadFile("exp_rule.txt")
	if err != nil {
		t.Fatal(err)
	}
	ruleInfos := funk.Map(strings.Split(string(content), "\n"), func(s string) [2]string {
		splits := strings.Split(s, "\x00")
		return [2]string{splits[1], splits[0]}
	})
	rules, err := ParseExpRule(ruleInfos.([][2]string))
	if err != nil {
		t.Fatal(err)
	}
	_ = rules
}

func TestExportExpRules(t *testing.T) {
	db, err := gorm.Open("sqlite3", "/Users/z3/Downloads/TideFinger/python3/cms_finger.db")
	if err != nil {
		t.Fatal(err)
	}
	db = db.Debug()
	raws, err := db.Table("tide").Rows()
	if err != nil {
		t.Fatal(err)
	}
	ruleMap := map[string]struct{}{}
	ruleList := [][]string{}
	for raws.Next() {
		var id int
		var name, keys string
		err := raws.Scan(&id, &name, &keys)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := ruleMap[name]; ok {
			continue
		}
		ruleMap[name] = struct{}{}
		ruleList = append(ruleList, []string{name, keys})
	}
	raws, err = db.Table("fofa_back").Rows()
	if err != nil {
		t.Fatal(err)
	}
	for raws.Next() {
		var id int
		var name, keys string
		err := raws.Scan(&id, &name, &keys)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := ruleMap[name]; ok {
			continue
		}
		ruleMap[name] = struct{}{}
		ruleList = append(ruleList, []string{name, keys})
	}

	res1 := funk.Map(ruleList, func(d []string) string {
		return strings.Join(d, "\x00")
	})
	res := strings.Join(res1.([]string), "\n")
	os.WriteFile("/Users/z3/Downloads/rule.txt", []byte(res), 0644)
	//println(res)
}
