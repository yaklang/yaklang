package ssa

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Const.String() is on the object-type field lookup path: getFieldIndexByKeyText
// compares every stored key with the requested text. Rendering each key with
// fmt.Sprintf("%v", value) turned one lookup over a large object type into
// hundreds of kilobytes of string formatting. On dotCMS ace-builds the lookup
// line held 74% of a 20s CPU window.
//
// The fast paths must not retain anything: newConstCreate deliberately dropped
// the cached formatted text because it kept a copy of every literal alive.
func TestConstStringFastPathsDoNotFormat(t *testing.T) {
	large := strings.Repeat("const-key-", 1<<17) // 1 MiB

	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"large string", large, large},
		{"short string", "field", "field"},
		{"int", 42, "42"},
		{"negative int", -7, "-7"},
		{"int64", int64(-1 << 40), "-1099511627776"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, NewConst(tc.value).String())
			require.Equal(t, fmt.Sprintf("%v", tc.value), NewConst(tc.value).String(),
				"fast path must keep the previous fmt %%v rendering")
		})
	}
}

// A 1 MiB string constant must render without copying it. fmt %v copies.
func TestConstStringDoesNotCopyLargeValue(t *testing.T) {
	const key = "const-string-no-copy-marker"
	c := NewConst(key)
	allocs := testing.AllocsPerRun(100, func() { _ = c.String() })
	require.Zero(t, allocs, "string constants must be returned as-is, got %v allocations", allocs)

	i := NewConst(int64(123456))
	allocs = testing.AllocsPerRun(100, func() { _ = i.String() })
	require.LessOrEqual(t, allocs, 1.0, "integers format with one allocation, got %v", allocs)
}

// Object type field lookup must not spend time formatting keys. The lookup is
// already memoized per key text; this guards the first (uncached) scan.
func TestObjectTypeFieldLookupWithLargeStringKeys(t *testing.T) {
	obj := NewObjectType()
	builderKeys := make([]Value, 0, 64)
	for i := 0; i < 64; i++ {
		obj.AddField(NewConst(fmt.Sprintf("field-%d", i)), CreateNumberType())
	}
	_ = builderKeys

	allocs := testing.AllocsPerRun(50, func() {
		obj.keymap = nil // force the linear scan, as a freshly built or merged type has
		require.NotNil(t, obj.GetField(NewConst("field-63")))
	})
	require.Less(t, allocs, 200.0, "field lookup must not format every key, got %v allocations", allocs)
}
