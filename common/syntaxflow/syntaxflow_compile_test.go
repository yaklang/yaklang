package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func TestCompile(t *testing.T) {
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	frame, err := vm.Compile(`a?{.abc}`)
	if err != nil {
		t.Fatal(err)
	}
	frame.Show()
}
