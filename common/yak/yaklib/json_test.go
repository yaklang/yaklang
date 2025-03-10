package yaklib

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJsonLoads(t *testing.T) {
	t.Run("omap", func(t *testing.T) {
		for i := 0; i < 1000; i++ {
			m := _jsonLoad(`{"b":2,"a":1,"c":3}`)
			v := _jsonDumps(m, _withIndent(""), _withPrefix(""))
			require.Equal(t, `{
"b": 2,
"a": 1,
"c": 3
}`, v)
		}
	})
}
