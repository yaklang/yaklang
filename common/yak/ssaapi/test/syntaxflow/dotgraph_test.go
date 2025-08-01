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

func TestDataFlowGraphEdgeLabel(t *testing.T) {
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
					{From: "funcA(a)", To: "55 + c", Label: "effect_on"},
					{From: "55 + c", To: "bbb", Label: "effect_on"},
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
					{From: "return a", To: "11", Label: "effect_on"},
					{From: "() => {\na = 11\nreturn a\n}", To: "return a", Label: "effect_on"},
					{From: "f()", To: "() => {\na = 11\nreturn a\n}", Label: "effect_on"},
					{From: "println(t)", To: "f()", Label: "effect_on"},
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
					{From: "return a", To: "11", Label: "effect_on"},
					{From: "() =>{\na = 11\nreturn a\n}", To: "return a", Label: "effect_on"},
					{From: "f()", To: "() =>{\na = 11\nreturn a\n}", Label: "effect_on"},
					{From: "f2(t)", To: "f()", Label: "effect_on"},
					{From: "i", To: "f2(t)", Label: "effect_on"},
					{From: "println(i)", To: "i", Label: "effect_on"},
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
			`a--> as $result`,
			map[string][]ssatest.EdgeInfo{
				"result": {
					{From: "return a", To: "11", Label: "effect_on"},
					{From: "() =>{\na = 11\nreturn a\n}", To: "return a", Label: "effect_on"},
					{From: "f()", To: "() =>{\na = 11\nreturn a\n}", Label: "effect_on"},
					{From: "f2(t)", To: "f()", Label: "effect_on"},
					{From: "i", To: "f2(t)", Label: "effect_on"},
					{From: "println(i)", To: "i", Label: "effect_on"},
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
