package yak

import (
	"yaklang/common/log"
	"yaklang/common/yak/yaklang"
	"testing"
)

func TestScriptEngine_Execute(t *testing.T) {
	eg := yaklang.New()
	err := eg.Eval(`func abc(a, b, c) {
die("which line?")
return true, true, true}; 
a, b = abc("123", "a", 1235)`)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
}
