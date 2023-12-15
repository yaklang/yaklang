package syntaxflow

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/omap"
	"testing"
)

func check(c string) {
	vm := sfvm.NewSyntaxFlowVirtualMachine[string, any]().Debug(true)
	err := vm.Compile(c)
	if err != nil {
		panic(err)
	}
	m := omap.NewEmptyOrderedMap[string, any]()
	m.Set("abc", "def")
	m.Set("search", "def")
	m.Set("fetch", map[string]any{
		"param": []any{
			"/abc",
		},
	})
	vm.Feed(m)
}

func TestSyntaxFlow_Basic(t *testing.T) {
	check(`$abc >> fetch => [param] => {header: 2.(dict); path: 1.(str); } => $request`)
}

func TestSyntaxFlow_Fetch(t *testing.T) {
	check(`fetch => [param] => {header: 2.(dict); path: 1.(str); } => $request`)
}
