package yak

import (
	"context"
	"testing"
	"time"

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

func TestScriptEngine_nativeCall_YakScript_smoking1(t *testing.T) {
	code := `
time.AfterFunc(2 ,func(){
    println(a)
})
`
	engine := NewScriptEngine(10)
	_, err := engine.ExecuteExWithContext(context.Background(), code, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
}
