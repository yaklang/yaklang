package yakunquote

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnquoteInvalidUTF8(t *testing.T) {
	raw := "你好"
	got, err := Unquote(`"\xE4\xBD\xA0\xE5\xA5\xBD"`)
	require.NoError(t, err)
	require.Equal(t, raw, got)
}

func TestUnquoteUnicode(t *testing.T) {
	raw := "你好"
	input := `"\u4F60\u597D"`
	got, err := Unquote(input)
	require.NoError(t, err)
	require.Equal(t, raw, got)
}
