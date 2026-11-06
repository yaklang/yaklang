package ssa

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// TestClipMemberVerboseNameKeepsShortNames guards the common case: a shallow
// member path must come back untouched.
func TestClipMemberVerboseNameKeepsShortNames(t *testing.T) {
	for _, name := range []string{"", "a", "this.field", "com.example.Foo.bar"} {
		require.Equal(t, name, clipMemberVerboseName(name))
	}
}

// TestClipMemberVerboseNameBoundsDeepPaths is the OOM guard: a member chain
// deep enough to repeat its ancestors thousands of times must still store a
// small string.
func TestClipMemberVerboseNameBoundsDeepPaths(t *testing.T) {
	deep := strings.Repeat("abcdefgh.", 2000)
	got := clipMemberVerboseName(deep)
	require.LessOrEqual(t, len(got), maxMemberVerboseNameLen+len("..."))
	require.True(t, strings.HasPrefix(got, "abcdefgh."), "the head identifies the object")
}

// Multi-byte names must not be cut mid-rune.
func TestClipMemberVerboseNameDoesNotSplitRunes(t *testing.T) {
	got := clipMemberVerboseName(strings.Repeat("中", 400))
	require.True(t, utf8.ValidString(got))
	require.LessOrEqual(t, len(got), maxMemberVerboseNameLen+len("..."))
}
