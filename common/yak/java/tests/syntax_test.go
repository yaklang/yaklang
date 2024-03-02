package tests

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"testing"
)

//go:embed allinone8.java
var allinone8 string

//go:embed allinone7.java
var allinone7 string

//go:embed allinone11.java
var allinone11 string

//go:embed allinone17.java
var allinone17 string

func TestJavaBasicParser_Java8(t *testing.T) {
	errs := java2ssa.ParserSSA(allinone8).GetErrors()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
}

func TestJavaBasicParser_Java7(t *testing.T) {
	errs := java2ssa.ParserSSA(allinone7).GetErrors()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
}

func TestJavaBasicParser_Java17(t *testing.T) {
	errs := java2ssa.ParserSSA(allinone17).GetErrors()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
}

func TestJavaBasicParser_Java11(t *testing.T) {
	errs := java2ssa.ParserSSA(allinone11).GetErrors()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
}
