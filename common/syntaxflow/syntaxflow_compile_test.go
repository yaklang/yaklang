package syntaxflow

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"testing"
)

func TestCompile(t *testing.T) {
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	err := vm.Compile(`a?{.abc}`)
	if err != nil {
		t.Fatal(err)
	}
	vm.Show()
}
