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
	prog, err := ssaapi.Parse(str, ssaapi.WithLanguage(ssaapi.JS))
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}
