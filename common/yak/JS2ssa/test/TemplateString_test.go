package test

import (
	_ "embed"
	"fmt"
	_ "net/http/pprof"
	"testing"
)

func TestTemplateString2(t *testing.T) {
	prog, err := ParseSSA("var a = `hello ${5 + 10} world`;")
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
	fmt.Println(prog.GetErrors())
}

func TestTemplateString3(t *testing.T) {
	prog, err := ParseSSA("var b = 123; var a = `b = ${b}`;")
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
	fmt.Println(prog.GetErrors())
}