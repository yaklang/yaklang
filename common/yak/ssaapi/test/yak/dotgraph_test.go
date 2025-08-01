package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSimpleDotGraphEdgeLabel(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		sfRule        string
		expectedEdges []ssatest.EdgeInfo
	}{
		{
			"simple call",
			"a()",
			"a() as $result",
			[]ssatest.EdgeInfo{
				{VariableName: "result", From: "a", To: "a()", Label: "call"},
			},
		},
		{
			"exact search",
			"a.b()",
			"a.b as $result",
			[]ssatest.EdgeInfo{
				{VariableName: "result", From: "a", To: ".b", Label: "search-exact"},
			},
		},
		{
			"glob search",
			"a.bb()",
			"a.b* as $result",
			[]ssatest.EdgeInfo{
				{VariableName: "result", From: "a", To: ".bb", Label: "search-glob:b*"},
			},
		},
		{
			"regex search",
			"a.bb()",
			"a./bb/ as $result",
			[]ssatest.EdgeInfo{
				{VariableName: "result", From: "a", To: ".bb", Label: "search-regexp:bb"},
			},
		},
		{
			"get user with dot graph edge label",
			"a.b()",
			"b-> as $result",
			[]ssatest.EdgeInfo{
				{VariableName: "result", From: ".b", To: "a.b()", Label: "step[1]: getUser"},
			},
		},
		{
			"get user by native call with dot graph edge label",
			"a.b()",
			"b<getUsers> as $result",
			[]ssatest.EdgeInfo{
				{VariableName: "result", From: ".b", To: "a.b()", Label: "native-call:[getUsers]"},
			},
		},
		{
			"deep chain test",
			"a.b.c.d.e.f.g.h().aaa.bbb.ccc()",
			`a...h as $result1;
				$result1...ccc() as $result2`,
			[]ssatest.EdgeInfo{
				{VariableName: "result1", From: "a", To: ".h", Label: "step[1]: recursive search h"},
				{VariableName: "result2", From: ".h", To: ".ccc", Label: "step[2]: recursive search ccc"},
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
