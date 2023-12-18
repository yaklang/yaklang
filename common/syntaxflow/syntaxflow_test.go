package syntaxflow

import (
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"sort"
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
		"body": map[string]any{},
	})
	return vm.Feed(m)
}

func jsonCheck(raw string, rule string) *omap.OrderedMap[string, any] {
	var i any = nil
	err := json.Unmarshal([]byte(raw), &i)
	if err != nil {
		panic(err)
	}
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	err = vm.Compile(rule)
	if err != nil {
		panic(err)
	}
	return vm.Debug(true).Feed(omap.BuildGeneralMap[any](i))
}

type checkCase struct {
	Data string
	Rule string
}

func TestJSONBuild_Map(t *testing.T) {
	c := checkCase{
		Data: `{"abc": "def", "bbc": "ccc", "body": {"a1bc": "1111"}}`,
		Rule: `%bc => {ddd: $} => $eee`,
	}
	result := jsonCheck(c.Data, c.Rule)
	resultMap := result.Field("eee")
	var v1 = resultMap.GetByIndexMust(0).(*omap.OrderedMap[string, any]).GetMust("ddd").([]any)[0]
	var v2 = resultMap.GetByIndexMust(1).(*omap.OrderedMap[string, any]).GetMust("ddd").([]any)[0]
	var v3 = resultMap.GetByIndexMust(2).(*omap.OrderedMap[string, any]).GetMust("ddd").([]any)[0]
	var generalResult = []any{v1, v2, v3}
	r := utils.InterfaceToStringSlice(generalResult)
	sort.SliceStable(r, func(i, j int) bool {
		return r[i] < r[j]
	})
	fmt.Println(r)
	assert.NotEqual(t, []string{"ccc", "def"}, r)
	assert.Equal(t, []string{"1111", "ccc", "def"}, r)
	assert.Len(t, r, 3)
}

func TestJSONBuild_Flat(t *testing.T) {
	c := checkCase{
		Data: `{"abc": "def", "bbc": "ccc", "body": {"a1bc": "1111"}}`,
		Rule: `%bc => [?(!=ccc)] => $ccc`,
	}
	result := jsonCheck(c.Data, c.Rule)
	resultMap := result.Field("ccc")
	v1, _ := resultMap.GetByIndex(0)
	v2, _ := resultMap.GetByIndex(1)
	var generalResult = []any{v1, v2}
	r := utils.InterfaceToStringSlice(generalResult)
	sort.SliceStable(r, func(i, j int) bool {
		return r[i] < r[j]
	})
	fmt.Println(r)
	assert.NotEqual(t, []string{"ccc", "def"}, r)
	assert.NotEqual(t, []string{"1111", "ccc", "def"}, r)
	assert.Equal(t, []string{"1111", "def"}, r)
	assert.Len(t, r, 2)
}

func TestJSONBuild_Filter(t *testing.T) {
	c := checkCase{
		Data: `{"abc": "def", "bbc": "ccc", "body": {"a1bc": "1111"}}`,
		Rule: `%bc?(!=ccc) => $ccc`,
	}
	result := jsonCheck(c.Data, c.Rule)
	r := utils.InterfaceToStringSlice(result.GetMust("ccc").([]any))
	sort.SliceStable(r, func(i, j int) bool {
		return r[i] < r[j]
	})
	fmt.Println(r)
	assert.NotEqual(t, []string{"ccc", "def"}, r)
	assert.NotEqual(t, []string{"1111", "ccc", "def"}, r)
	assert.Equal(t, []string{"1111", "def"}, r)
	assert.Len(t, r, 2)
}

func TestJSONBuild_1(t *testing.T) {
	c := checkCase{
		Data: `{"abc": "def"}`,
		Rule: `%bc => $ccc`,
	}
	result := jsonCheck(c.Data, c.Rule)
	assert.Equal(t, "def", result.GetMust("ccc").([]any)[0])
}

func TestJSONBuild_3_Deep(t *testing.T) {
	c := checkCase{
		Data: `{"abc": "def", "bbc": "ccc", "body": {"a1bc": "1111"}}`,
		Rule: `%bc => $ccc`,
	}
	result := jsonCheck(c.Data, c.Rule)
	r := utils.InterfaceToStringSlice(result.GetMust("ccc").([]any))
	sort.SliceStable(r, func(i, j int) bool {
		return r[i] < r[j]
	})
	fmt.Println(result)
	assert.NotEqual(t, []string{"ccc", "def"}, r)
	assert.Equal(t, []string{"1111", "ccc", "def"}, r)
}

func TestJSONBuild_3_Deep2(t *testing.T) {
	c := checkCase{
		Data: `{"abc": "def", "bbc": "ccc", "body": {"abc": "1111"}}`,
		Rule: `%bc => $ccc`,
	}
	result := jsonCheck(c.Data, c.Rule)
	r := utils.InterfaceToStringSlice(result.GetMust("ccc").([]any))
	sort.SliceStable(r, func(i, j int) bool {
		return r[i] < r[j]
	})
	fmt.Println(r)
	assert.NotEqual(t, []string{"ccc", "def"}, r)
	assert.Equal(t, []string{"1111", "ccc", "def"}, r)
}

func TestJSONBuild_2(t *testing.T) {
	c := checkCase{
		Data: `{"abc": "def", "bbc": "ccc"}`,
		Rule: `%bc => $ccc`,
	}
	result := jsonCheck(c.Data, c.Rule)
	r := utils.InterfaceToStringSlice(result.GetMust("ccc").([]any))
	sort.SliceStable(r, func(i, j int) bool {
		return r[i] < r[j]
	})
	spew.Dump(r)
	assert.Equal(t, []string{"ccc", "def"}, r)
}

func TestBuildMap_Basic2(t *testing.T) {
	result := check(`fetch => {path: param[0]; method: options.method} => $reqs`)
	_ = result
	log.Info("finished")
	reqs := result.Field("reqs")
	e, ok := reqs.GetByIndex(0)
	if !ok {
		t.Error("reqs[0] not found")
		t.FailNow()
	}
	result = e.(*omap.OrderedMap[string, any])
	p, ok := result.Get("path")
	if !ok {
		t.FailNow()
	}
	if p != "/abc" {
		t.Fatal("path != /abc")
	}

	p, ok = result.Get("method")
	if !ok {
		t.FailNow()
	}
	if p != "GET" {
		t.Fatal("method failed")
	}
}

func TestBuildMap_Basic(t *testing.T) {
	result := check(`fetch => {path: param[0];} => $reqs`)
	_ = result
	log.Info("finished")
	reqs := result.Field("reqs")
	e, ok := reqs.GetByIndex(0)
	if !ok {
		t.Error("reqs[0] not found")
		t.FailNow()
	}
	p, ok := e.(*omap.OrderedMap[string, any]).Get("path")
	if !ok {
		t.FailNow()
	}
	if p != "/abc" {
		t.Fatal("path != /abc")
	}
}

func TestField(t *testing.T) {
	result := check(`fetch.value => $value`)
	if result, ok := result.Field("value").GetByIndex(0); ok {
		if result != "ccc" {
			t.Fatal("fetch.value != ccc")
		}
	} else {
		t.Error("fetch.value not found")
	}
}

func TestSyntaxFlow_DotFilter(t *testing.T) {
	result := check(`fetch.param[0] => $path`)
	target, ok := result.Field("path").GetByIndex(0)
	if !ok {
		t.Error("fetch.param[0] not found")
	}
	if target != "/abc" {
		t.Fatal("fetch.param[0] != /abc")
	}
}

func TestSyntaxFlow_Ref(t *testing.T) {
	result := check(`fetch => $ref`)
	res, ok := result.Field("ref").Get("param")
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
