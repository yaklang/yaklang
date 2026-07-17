package loopinfra

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLooksLikeCodePatch(t *testing.T) {
	assert.True(t, LooksLikeCodePatch("*** Begin Patch\n*** End Patch"))
	assert.False(t, LooksLikeCodePatch("x = 1\ny = 2\n"))
	assert.False(t, LooksLikeCodePatch(""))
}

func TestParseAndApply_SingleHunkReplace(t *testing.T) {
	full := "a = 1\nb = 2\nc = 3\n"
	patch := `*** Begin Patch
*** Update File: demo.yak
@@ replace b
 a = 1
-b = 2
+b = 42
 c = 3
*** End Patch`
	hunks, err := ParseCodePatch(patch)
	require.NoError(t, err)
	require.Len(t, hunks, 1)
	assert.Equal(t, "a = 1\nb = 2\nc = 3", hunks[0].OldText)
	assert.Equal(t, "a = 1\nb = 42\nc = 3", hunks[0].NewText)

	out, err := ApplyCodePatch(full, hunks)
	require.NoError(t, err)
	assert.Equal(t, "a = 1\nb = 42\nc = 3\n", out)
}

func TestParseAndApply_MultiHunk(t *testing.T) {
	full := "one\ntwo\nthree\nfour\n"
	patch := `*** Begin Patch
*** Update File: x.yak
@@ first
-one
+ONE
@@ second
-three
+THREE
*** End Patch`
	out, err := ApplyCodePatchFromString(full, patch)
	require.NoError(t, err)
	assert.Equal(t, "ONE\ntwo\nTHREE\nfour\n", out)
}

func TestApply_NotFound(t *testing.T) {
	full := "a = 1\n"
	patch := `*** Begin Patch
@@ miss
-missing
+x
*** End Patch`
	_, err := ApplyCodePatchFromString(full, patch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestApply_Ambiguous(t *testing.T) {
	full := "x=1\nfoo\nx=1\n"
	patch := `*** Begin Patch
@@ ambig
-x=1
+x=9
*** End Patch`
	_, err := ApplyCodePatchFromString(full, patch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "matched")
}

func TestApply_NormalizesLineEndingsAndTrailingWhitespace(t *testing.T) {
	full := "before\r\nx = 1  \r\nafter\r\n"
	patch := `*** Begin Patch
@@ normalize
 before
-x = 1
+x = 42
 after
*** End Patch`

	out, err := ApplyCodePatchFromString(full, patch)
	require.NoError(t, err)
	assert.Equal(t, "before\nx = 42\nafter\r\n", out)
}

func TestApply_NormalizedMatchStillRequiresUniqueness(t *testing.T) {
	full := "x = 1  \r\nmiddle\r\nx = 1\t\r\n"
	patch := `*** Begin Patch
@@ ambiguous after normalization
-x = 1
+x = 2
*** End Patch`

	_, err := ApplyCodePatchFromString(full, patch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "matched 2 times")
}

func TestApply_DeletionHunk(t *testing.T) {
	full := "keep\nDROP_ME\nkeep2\n"
	patch := `*** Begin Patch
@@ delete
 keep
-DROP_ME
 keep2
*** End Patch`
	out, err := ApplyCodePatchFromString(full, patch)
	require.NoError(t, err)
	assert.Equal(t, "keep\nkeep2\n", out)
}

func TestApply_InsertViaContext(t *testing.T) {
	full := "before\nafter\n"
	patch := `*** Begin Patch
@@ insert
 before
+middle
 after
*** End Patch`
	out, err := ApplyCodePatchFromString(full, patch)
	require.NoError(t, err)
	assert.Equal(t, "before\nmiddle\nafter\n", out)
}

func TestParse_EmptyPatch(t *testing.T) {
	_, err := ParseCodePatch("")
	require.Error(t, err)

	_, err = ParseCodePatch("*** Begin Patch\n*** End Patch")
	require.Error(t, err)
}

func TestSummarizeAppliedPatch_NoBeginMarker(t *testing.T) {
	hunks := []CodePatchHunk{{Header: "x", NewText: "hello world"}}
	s := SummarizeAppliedPatch(hunks)
	assert.Contains(t, s, "applied 1 patch hunk")
	assert.NotContains(t, s, "*** Begin Patch")
	assert.True(t, strings.Contains(s, "hello") || strings.Contains(s, "hunk"))
}
