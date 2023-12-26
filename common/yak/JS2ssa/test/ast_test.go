package test

import (
	_ "embed"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

//go:embed test.js
var str string

func TestRealJs(t *testing.T) {
	fmt.Println(len(str))
	prog := ssaapi.Parse(str, ssaapi.WithLanguage(ssaapi.JS))
	if prog.IsNil() {
		t.Fatal("parse js err")
	}
	prog.Show()
}
