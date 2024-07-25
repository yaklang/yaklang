package limitedmap

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestReadOnlyMap_Append(t *testing.T) {
	m := NewReadOnlyMap(map[string]any{
		"a": map[string]any{
			"b": 1,
			"c": 2,
		},
	})
	nm := m.Append(map[string]any{
		"a": map[string]any{
			"d": 3,
		},
	})
	assert.NotNil(t, nm.parent)
	raw, ok := nm.Load("a")
	assert.True(t, ok)
	assert.Equal(t, len(raw.(map[string]any)), 3)
}

func TestSafeMap_Append(t *testing.T) {
	m := NewSafeMap(map[string]any{
		"a": map[string]any{
			"b": 1,
			"c": 2,
		},
	})
	nm := m.Append(map[string]any{
		"a": map[string]any{
			"d": 3,
		},
	})
	assert.Nil(t, nm.parent)
	raw, ok := nm.Load("a")
	assert.True(t, ok)
	assert.Equal(t, len(raw.(map[string]any)), 3)
}

func TestSafeMap_Parent(t *testing.T) {
	rm := NewReadOnlyMap(map[string]any{
		"a": map[string]any{
			"global": 1,
		},
	})
	m := NewSafeMap(map[string]any{
		"a": map[string]any{
			"b": 1,
			"c": 2,
		},
	})
	m.SetPred(rm)
	nm := m.Append(map[string]any{
		"a": map[string]any{
			"d": 3,
		},
	})
	assert.NotNil(t, nm.parent)
	assert.Equal(t, nm.parent, rm)

	raw, ok := nm.Load("a")
	assert.True(t, ok)
	assert.Equal(t, len(raw.(map[string]any)), 4)
}
