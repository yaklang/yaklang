package syntaxflow

import (
	"github.com/davecgh/go-spew/spew"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func TestSyntaxFlowFilter_Search(t *testing.T) {
	for _, i := range []map[string]string{
		{
			"code":    "exec", // Ref
			"keyword": "push$exact",
		},
		{
			"code":    "ex*",
			"keyword": "push$glob",
		},
		{
			"code":    "/abc/",
			"keyword": "push$regex",
		},
	} {
		vm := sfvm.NewSyntaxFlowVirtualMachine()
		vm.Debug()
		frame, err := vm.Compile(i["code"])
		if err != nil {
			t.Fatal(err)
		}
		vm.Show()
		count := 0
		checked := false
		count += len(frame.Codes)
		for _, c := range frame.Codes {
			if strings.Contains(c.String(), i["keyword"]) {
				checked = true
			}
		}
		if !checked {
			t.Fatalf("SyntaxFlowVirtualMachine.Compile failed: %v", spew.Sdump(i))
		}
	}

}

func TestCompile(t *testing.T) {
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	frame, err := vm.Compile(`a?{.abc}`)
	if err != nil {
		t.Fatal(err)
	}
	frame.Show()
}
