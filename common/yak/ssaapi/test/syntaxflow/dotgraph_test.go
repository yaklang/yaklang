package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSimpleDotGraphEdgeLabel(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		sfRule        string
		expectedEdges map[string][]ssatest.EdgeInfo
	}{
		{
			"simple call",
			"a()",
			"a() as $result",
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: "a", To: "a()", Label: "call"},
				},
			},
		},
		{
			"exact search",
			"a.b()",
			"a.b as $result",
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: "a", To: ".b", Label: "search-exact"},
				},
			},
		},
		{
			"glob search",
			"a.bb()",
			"a.b* as $result",
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: "a", To: ".bb", Label: "search-glob:b*"},
				},
			},
		},
		{
			"regex search",
			"a.bb()",
			"a./bb/ as $result",
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: "a", To: ".bb", Label: "search-regexp:bb"},
				},
			},
		},
		{
			"get user with dot graph edge label",
			"a.b()",
			"b-> as $result",
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: ".b", To: "a.b()", Label: "step[1]: getUser"},
				},
			},
		},
		{
			"get user by native call with dot graph edge label",
			"a.b()",
			"b<getUsers> as $result",
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: ".b", To: "a.b()", Label: "native-call:[getUsers]"},
				},
			},
		},
		{
			"deep chain test",
			"a.b.c.d.e.f.g.h().aaa.bbb.ccc()",
			`a...h as $result1;
				$result1...ccc() as $result2`,
			map[string][]ssatest.EdgeInfo{
				"result1": {
					{From: "a", To: ".h", Label: "step[1]: recursive search h"},
				},
				"result2": {
					{From: ".h", To: ".ccc", Label: "step[2]: recursive search ccc"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ssatest.CheckSyntaxFlowDotGraph(
				t,
				tt.code,
				tt.sfRule,
				true,
				tt.expectedEdges,
			)
		})
	}
}

func TestFilterRuleDotGraphEdgeLabel(t *testing.T) {
	t.Skip("图上是否显示有过滤不影响路径信息，感觉不太需要加过滤的表示")
	tests := []struct {
		name          string
		code          string
		sfRule        string
		expectedEdges map[string][]ssatest.EdgeInfo
	}{
		{
			"compare string",
			`a1()
				a2()`,
			`a*?{have:"a1"} as $result`,
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: "a", To: "a()", Label: "call"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ssatest.CheckSyntaxFlowDotGraph(
				t,
				tt.code,
				tt.sfRule,
				true,
				tt.expectedEdges,
			)
		})
	}
}

func TestBottomUseGraphEdgeLabel(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		sfRule        string
		expectedEdges map[string][]ssatest.EdgeInfo
	}{
		{
			"basic bottom use",
			`var c = bbb
	var a = 55 + c
	funcA(a)`,
			`c --> as $result`,
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: "bbb", To: "55 + c", Label: "depend_on"},
					{From: "55 + c", To: "funcA(a)", Label: "depend_on"},
				},
			},
		},
		{
			"bottom-use:simple cross process 1",
			`
		f = () => {
			a = 11
			return a
		}
		t = f()
		println(t)
		`,
			`a --> as $result`,
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: "11", To: "return a", Label: "depend_on"},
					{From: "return a", To: "() => {\na = 11\nreturn a\n}", Label: "depend_on"},
					{From: "() => {\na = 11\nreturn a\n}", To: "f()", Label: "depend_on"},
					{From: "f()", To: "println(t)", Label: "depend_on"},
				},
			},
		},
		{
			"bottom-use:simple cross process 2",
			`
		f = () =>{
			a = 11
			return a
		}
		f2 = (i) => {
			println(i)
		}
		t = f()
		f2(t)
		`,
			`a --> as $result`,
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: "11", To: "return a", Label: "depend_on"},
					{From: "return a", To: "() =>{\na = 11\nreturn a\n}", Label: "depend_on"},
					{From: "() =>{\na = 11\nreturn a\n}", To: "f()", Label: "depend_on"},
					{From: "f()", To: "f2(t)", Label: "depend_on"},
					{From: "f2(t)", To: "i", Label: "depend_on"},
					{From: "i", To: "println(i)", Label: "depend_on"},
				},
			},
		},
		{
			"bottom-use:side effect 1",
			`
		a = 11
		b = () => {
			a = 22
		}
		b()
		println(a)
		`,
			`a?{=11}-->  as $result`,
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: "11", To: "a = 22", Label: "depend_on"},
					{From: "11", To: "b()", Label: "depend_on"},
					{From: "b()", To: "println(a)", Label: "depend_on"},
				},
			},
		},
		{
			"bottom-use:side effect 2",
			`
		o = 5
		a = o
		b = () => {
			a = 2
		}
		if e {b()}
		c = a+1;
		`,
			`o?{=5}-->  as $result`,
			map[string][]ssatest.EdgeInfo{
				"result": {
					// phi
					{From: "if e {b()}", To: "a+1", Label: "depend_on"},
					{From: "if e {b()}", To: "a+1", Label: "depend_on"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ssatest.CheckSyntaxFlowDotGraph(
				t,
				tt.code,
				tt.sfRule,
				true,
				tt.expectedEdges,
			)
		})
	}
}

func TestTopDefGraphEdgeLabel(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		sfRule        string
		expectedEdges map[string][]ssatest.EdgeInfo
	}{
		{
			"basic topdef",
			`f = (i) => {return i}
				a = f(333333)`,
			`a #-> as $result`,
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: "333333", To: "i", Label: "depend_on"},
					{From: "i", To: "(i) => {return i}", Label: "depend_on"},
					{From: "(i) => {return i}", To: "f(333333)", Label: "depend_on"},
				},
			},
		},
		{
			"test topdef:test level1 object",
			`f = () => {return {"key":"value"}}
		obj = f()
		a = obj.key`,
			`a #-> as $result`,
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: "f()", To: ".key", Label: "depend_on"},
					{From: "\"value\"", To: ".key", Label: "depend_on"},
					{From: "() => {return {\"key\":\"value\"}}", To: "f()", Label: "depend_on"},
					{From: "{\"key\":\"value\"}", To: "() => {return {\"key\":\"value\"}}", Label: "depend_on"},
				},
			},
		},
		{
			"test topdef: test level2 simple",
			`
		f = (i) => {
			return () => {return i} 
		}
		f1 = f(333333)
		a = f1()
		`,
			`a #-> as $result`,
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: "333333", To: "i", Label: "depend_on"},
					{From: "i", To: "() => {return i}", Label: "depend_on"},
					{From: "() => {return i}", To: "f1()", Label: "depend_on"},
					{From: "f(333333)", To: "() => {return i}", Label: "depend_on"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ssatest.CheckSyntaxFlowDotGraph(
				t,
				tt.code,
				tt.sfRule,
				true,
				tt.expectedEdges,
			)
		})
	}
}
