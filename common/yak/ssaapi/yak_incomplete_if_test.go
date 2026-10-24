package ssaapi

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Language completion parses unfinished conditions with syntax recovery enabled.
// Building a file's exports must retain the valid prefix in these programs.
func TestParseYakIncompleteIf(t *testing.T) {
	for _, tc := range []struct {
		name string
		code string
	}{
		{"string-member", "name = \"hello\"\nif name."},
		{"and-chain", `if str.HasPrefix("a", "b") && poc.`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := Parse(tc.code, WithMemory(), WithIgnoreSyntaxError(true),
				WithExternLib("str", map[string]any{"HasPrefix": strings.HasPrefix}))
			require.NoError(t, err)
			require.NotNil(t, prog)
			if tc.name == "string-member" {
				values := prog.Ref("name")
				require.Len(t, values, 1)
				require.Equal(t, ssa.StringTypeKind, values[0].GetTypeKind())
			}
		})
	}
}
