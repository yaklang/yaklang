package yak

import (
	"context"
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func TestScriptEngine_Execute(t *testing.T) {
	eg := yaklang.New()
	err := eg.Eval(context.Background(), `func abc(a, b, c) {
die("which line?")
return true, true, true}; 
a, b = abc("123", "a", 1235)`)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
}

func TestA(t *testing.T) {
	code, _ := os.ReadFile("./a.yak")
	e := NewScriptEngine(10)
	e.Execute(string(code))
}
