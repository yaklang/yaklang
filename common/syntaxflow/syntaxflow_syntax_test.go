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
		`$(id)>> $`,
		`$(id)<< $`,
		`$id>> $`,
		"$i<< $",
		"$i<< $ =>$d",
		"$",
		"exec",    // Ref
		"1",       // *NumberLiteral
		".member", // Field
		">> $",    // Change Direction
		".*exec*",
		"*exec",
		"exe*c",
		"/$reexc/",
		"./$reexc/",
		"a[1]",
		"a.b",
		"c.d",
		"a[1]",
		"b?(!1)",
		"b?(>1)",
		"b?(!/abc/)",
	} {
		checkSyntax(i, t)
	}
}
