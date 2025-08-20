package ssaapi_test

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestValuesGraphInfoSimple(t *testing.T) {
	t.Run("test graph info simple 1", func(t *testing.T) {
		code := `a()`
		rule := `a() as $result`
		ssatest.CheckSyntaxFlowGraphInfo(
			t,
			code,
			rule,
			"result",
			map[string]ssatest.GraphNodeInfo{
				"n1": {
					Label: "Undefined-a()",
					CodeRange: &ssaapi.CodeRange{
						URL:            "/",
						StartLine:      1,
						StartColumn:    1,
						EndLine:        1,
						EndColumn:      4,
						SourceCodeLine: 0,
					},
				},
				"n2": {
					Label: "Undefined-a",
					CodeRange: &ssaapi.CodeRange{
						URL:            "/",
						StartLine:      1,
						StartColumn:    1,
						EndLine:        1,
						EndColumn:      2,
						SourceCodeLine: 0,
					},
				},
			},
			[][]string{
				{"n1", "n2"},
			},
		)
	})
}
