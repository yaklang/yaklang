package java2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJavaLiteralEscapes(t *testing.T) {
	for _, tc := range []struct{ literal, want string }{
		{`'\0'`, "\x00"}, {`'\07'`, "\a"}, {`'\377'`, "ÿ"},
		{`"\400"`, " 0"}, {`"\777"`, "?7"},
		{`"\\0"`, `\0`}, {`'\s'`, " "}, {`'\n'`, "\n"},
		{`'\u0041'`, "A"}, {`'中'`, "中"},
	} {
		t.Run(tc.literal, func(t *testing.T) {
			got, err := unquoteJavaLiteral(tc.literal)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
	_, err := unquoteJavaLiteral(`'\8'`)
	require.Error(t, err)
}
