package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestDotGraphEdgeLabel(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		sfRule        string
		expectedEdges []ssatest.EdgeInfo
	}{
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
