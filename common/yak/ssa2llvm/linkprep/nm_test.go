package linkprep

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseNmDefined_skipsArchiveHeaders(t *testing.T) {
	raw := `go.o:
0000000000000070 T yak_runtime_invoke
0000000000000370 T yak_internal_malloc
`
	m := parseNmDefined(raw)
	require.True(t, m["yak_runtime_invoke"])
	require.True(t, m["yak_internal_malloc"])
	require.Len(t, m, 2)
}

func TestParseNmDefined_posixStyle(t *testing.T) {
	// Hypothetical `nm -P` style: name type value [size]
	raw := `yak_internal_malloc T 0 0
other_sym D 0x10 8
`
	m := parseNmDefined(raw)
	require.True(t, m["yak_internal_malloc"])
	require.True(t, m["other_sym"])
}

func TestParseNmDefined_ignoresUndefinedStyle(t *testing.T) {
	raw := `0000000000000000 U missing_sym
0000000000000010 T real_sym
`
	m := parseNmDefined(raw)
	require.False(t, m["missing_sym"])
	require.True(t, m["real_sym"])
}

func TestParseNmSymbolNames_undefinedTwoColumn(t *testing.T) {
	raw := `U yak_internal_release_shadow
0000000000000010 T yak_runtime_invoke
`
	m := parseNmSymbolNames(raw)
	require.True(t, m["yak_internal_release_shadow"])
	require.True(t, m["yak_runtime_invoke"])
}
