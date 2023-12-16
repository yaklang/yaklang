package syntaxflow

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"strings"
	"testing"
)

func check(c string) *omap.OrderedMap[string, any] {
	vm := sfvm.NewSyntaxFlowVirtualMachine().Debug(true)
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
		"value": "ccc",
		"options": map[string]any{
			"method": "GET",
			"headers": map[string][]string{
				"abc": []string{"ab", "c"},
			},
		},
	})
	return vm.Feed(m)
}

func TestBuildMap(t *testing.T) {
	result := check(`fetch => {path: param[0];} => $result`)
	_ = result
	log.Info("finished")
	e := result.Index(0)
	p, ok := e.Get("path")
	if !ok {
		t.FailNow()
	}
	spew.Dump(p)

	p, ok = e.Get("method")
	if !ok {
		t.FailNow()
	}
	spew.Dump(p)
}

func TestField(t *testing.T) {
	result := check(`fetch.value`)
	if result, ok := result.GetByIndex(0); ok {
		if result != "ccc" {
			t.Fatal("fetch.value != ccc")
		}
	} else {
		t.Error("fetch.value not found")
	}
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

func TestSyntaxFlow_Ref(t *testing.T) {
	result := check(`fetch`)
	res, ok := result.Get("param")
	if !ok {
		t.Error("fetch.param not found")
		panic(1)
	}
	if strings.Join(utils.InterfaceToStringSlice(res), "") != "/abc" {
		panic(111)
	}
}

//func TestSyntaxFlow_Basic(t *testing.T) {
//	check(`$abc >> fetch => [param, options] => {path: 0.(str); header: 1.(dict); } => $request`)
//}
//
//func TestSyntaxFlow_Fetch(t *testing.T) {
//	check(`fetch => [param] => {header: 2.(dict); path: 1.(str); } => $request`)
//}
