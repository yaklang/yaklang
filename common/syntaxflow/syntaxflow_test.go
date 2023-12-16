package syntaxflow

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/omap"
	"testing"
)

func check(c string) *omap.OrderedMap[string, any] {
	vm := sfvm.NewSyntaxFlowVirtualMachine[any]().Debug(true)
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
		"options": map[string]any{
			"method": "GET",
			"headers": map[string][]string{
				"abc": []string{"ab", "c"},
			},
		},
	})
	return vm.Feed(m)
}

func TestSyntaxFlow_DotFilter(t *testing.T) {
	result := check(`fetch.param[0]`)
	target, ok := result.GetByIndex(0)
	if !ok {
		t.Error("fetch.param[0] not found")
	}
	if target != "/abc" {
		t.Fatal("fetch.param[0] != /abc")
	}
}

func TestSyntaxFlow_Basic(t *testing.T) {
	check(`$abc >> fetch => [param, options] => {path: 0.(str); header: 1.(dict); } => $request`)
}

func TestSyntaxFlow_Fetch(t *testing.T) {
	check(`fetch => [param] => {header: 2.(dict); path: 1.(str); } => $request`)
}
