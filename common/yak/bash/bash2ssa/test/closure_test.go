package test

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestBashPrintlnClosure(t *testing.T) {
	ssatest.CheckPrintlnValueContain(sink+`x=outer; capture(){ println "$x"; }; capture`, []string{"FreeValue-x"}, t)
	ssatest.CheckPrintlnValue(sink+`x=outer; f(){ local x=inner; println "$x"; }; f; println "$x"`, []string{`"inner"`, `"outer"`}, t)
	ssatest.CheckPrintlnValue(sink+`x=outer; out=$(x=inner; println "$x"); println "$x"`, []string{`"inner"`, `"outer"`}, t)
	ssatest.CheckPrintlnValueRegexp(sink+`x=parent; out=$(x=child; get "$x"); println "$x"; println "$out"; { x=group; }; ( x=sub ); println "$x"`, []string{`^"parent"$`, `get.*"child"`, `^"group"$`}, t)
	ssatest.CheckPrintlnValue(sink+`x=parent; show "$x" | { x=pipeline; }; x=temporary show arg; println "$x"`, []string{`"parent"`}, t)
}
