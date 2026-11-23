package omap

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestUnmarshalJSON(t *testing.T) {
	m := NewEmptyOrderedMap[string, any]()
	m.UnmarshalJSON([]byte(`{"a":1.1,"b":2.2}`))
	assert.Equal(t, 1.1, GetMust(m, "a"))
	assert.Equal(t, 2.2, GetMust(m, "b"))
}
