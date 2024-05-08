package syntaxflow

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"testing"
)

func checkSyntax(i string, t *testing.T) {
	vm := sfvm.NewSyntaxFlowVirtualMachine().Debug(true)
	err := vm.Compile(i)
	if err != nil {
		t.Fatalf("syntax failed: %#v, reason: %v", i, err)
	}
}

func TestSyntaxInOne(t *testing.T) {
	for _, i := range []string{
		"$",
		"exec",    // Ref
		".member", // Field
		".*exec*",
		"*exec",
		"exe*c",
		"/$reexc/",
		"./$reexc/",
		"a[1]",
		"a.b",
		"c.d",
		"a[1]",
		"b?{!1}",
		"b?{>1}",
		"b?{!/abc/}",
		"/(?i)runtime/.exec(,,,#*exec)",
		"exec as $rough",
		"/(?i)runtime/.exec(,,,#*exec) as $a",
		"/(?i)runtime/.exec(,,,#*exec,,) as $a",
		"/(?i)runtime/.exec(,,,#*exec,,,) as $a",
		"a->b",
		"a#>b",
		"a-->b",
		"a#->b",
		"a->b->c",
		"a#>b#>c",
		"a-->b.exec()-->c",
		"a#->b.exec()#->c",
		"a-{}->b",
		"a#{}->b",
		"system(#get)",
		"system(#*)",
		"system(#a*get)",
		"a*",
		"*a",
		"*",
		"system(#{}*a)",
		"system(#{}*)",
		"a-{depth:1}->b",
		"a#{depth:1}->b",
		"a-{depth:1, " + "\nkey:value}->b",
		"a#{depth:1, " + "\nkey:value}->b",
		"a-{depth:1, " + "\nkey:value,}->b",
		"a#{depth:1, " + "\nkey:value,}->b",
	} {
		checkSyntax(i, t)
	}
}
