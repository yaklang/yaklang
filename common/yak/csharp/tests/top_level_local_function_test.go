package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCSharp_TopLevelLocalFunctionDataFlow(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "forward declaration",
			code: `
sink(Handle(source()));
object Handle(object value) { return value; }
`,
		},
		{
			name: "declaration first",
			code: `
object Handle(object value) { return value; }
sink(Handle(source()));
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prog := parseCSharpSemantics(t, test.code)
			flow, err := prog.SyntaxFlowWithError(`sink(* #-> as $origin)`)
			require.NoError(t, err)
			require.Contains(t, flow.GetValues("origin").String(), "source",
				"top-level local-function calls must bind to the shared declaration shell")
		})
	}
}
