package yaklib

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJsonLoads(t *testing.T) {
	t.Run("omap", func(t *testing.T) {
		m := _jsonLoad(`{"a":1,"b":2,"c":3}`)
		for i := 0; i < 100; i++ {
			v := _jsonDumps(m, _withIndent(""), _withPrefix(""))
			require.Equal(t, `{
"a": 1,
"b": 2,
"c": 3
}`, v)
		}
	})
}
