package omap

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestUnmarshalJSON(t *testing.T) {
	omap := NewEmptyOrderedMap[string, any]()
	omap.UnmarshalJSON([]byte(`{"a":1.1,"b":2.2}`))
	assert.Equal(t, 1.1, omap.GetMust("a"))
	assert.Equal(t, 2.2, omap.GetMust("b"))
}
