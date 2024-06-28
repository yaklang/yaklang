package parsers

import (
	"testing"
)

func _TestCompilerSpecialSyntax(t *testing.T) {
	rules, err := ParseExpRule(newTestGenerateRule(`header=""MiniCMS""`))
	if err != nil {
		t.Fatal(err)
	}
	rules, err = ParseExpRule(newTestGenerateRule(` body="VALUE="Copyright (C) 2000, Cobalt Networks"`))
	if err != nil {
		t.Fatal(err)
	}
	rules, err = ParseExpRule(newTestGenerateRule(`(body="Everything.gif"||body="everything.png") && title=="Everything"`))
	if err != nil {
		t.Fatal(err)
	}
	rules, err = ParseExpRule(newTestGenerateRule(`body="xheditor_lang/zh-cn.js"||body="class="xheditor"||body=".xheditor("`))
	if err != nil {
		t.Fatal(err)
	}
	rules, err = ParseExpRule(newTestGenerateRule(`server="TornadoServer"&&Celery`))
	if err != nil {
		t.Fatal(err)
	}
	_ = rules
}
func TestCompiler(t *testing.T) {
	rules, err := ParseExpRule(newTestGenerateRule(`header="\"MiniCMS\""`))
	if err != nil {
		t.Fatal(err)
	}
	rules, err = ParseExpRule(newTestGenerateRule("header=\"MiniCMS\" || title=\"MiniCMS\""))
	if err != nil {
		t.Fatal(err)
	}
	_ = rules
}
