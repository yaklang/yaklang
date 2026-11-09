package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// stripProgramNamePrefixSplit is the reference implementation: split the whole
// path, then rejoin. StripProgramNamePrefix must stay byte-for-byte compatible
// with it while avoiding the slice allocation.
func stripProgramNamePrefixSplit(filePath, programName string) string {
	if filePath == "" || programName == "" {
		return filePath
	}
	trimmed := strings.TrimPrefix(filePath, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return filePath
	}
	first := parts[0]
	if first == programName || strings.HasPrefix(first, programName+"(") {
		if len(parts) > 1 {
			return "/" + strings.Join(parts[1:], "/")
		}
		return "/"
	}
	return filePath
}

func TestStripProgramNamePrefixMatchesSplitImplementation(t *testing.T) {
	paths := []string{
		"",
		"/",
		"//",
		"///",
		"prog",
		"/prog",
		"/prog/",
		"/prog/a",
		"/prog/a/b.java",
		"/prog(2026-01-01)/a/b.java",
		"/prog(",
		"/prog(2026",
		"/progx/a",
		"/proga(1)/b",
		"/other/prog/a",
		"prog/a/b.java",
		"prog//a",
		"//prog/a",
		"/prog//a",
		"/prog/./a",
		"/PROG/a",
		"/prog prog/a",
		"a/b/c",
		"/a",
	}
	programs := []string{"", "prog", "prog(2026-01-01)", "other", "a", "PROG"}
	for _, programName := range programs {
		for _, filePath := range paths {
			want := stripProgramNamePrefixSplit(filePath, programName)
			got := StripProgramNamePrefix(filePath, programName)
			require.Equal(t, want, got, "filePath=%q programName=%q", filePath, programName)
		}
	}
}

func TestStripProgramNamePrefixCommonCases(t *testing.T) {
	require.Equal(t, "/a/b.java", StripProgramNamePrefix("/prog/a/b.java", "prog"))
	require.Equal(t, "/a/b.java", StripProgramNamePrefix("/prog(123)/a/b.java", "prog"))
	require.Equal(t, "/", StripProgramNamePrefix("/prog", "prog"))
	require.Equal(t, "/other/a", StripProgramNamePrefix("/other/a", "prog"))
	require.Equal(t, "/progx/a", StripProgramNamePrefix("/progx/a", "prog"))
	require.Equal(t, "a/b", StripProgramNamePrefix("a/b", "prog"))
}

// The fast path must not allocate at all when the prefix does not match: this
// function runs once per path filter check on large scans.
func TestStripProgramNamePrefixNoAllocOnMiss(t *testing.T) {
	got := testing.AllocsPerRun(100, func() {
		_ = StripProgramNamePrefix("/dotCMS/src/main/java/A.java", "other-program")
	})
	require.Zero(t, got, "non-matching path must not allocate")
}
