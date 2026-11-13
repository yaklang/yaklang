package memedit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// getInstructionFilePath asks for the file path once per instruction while rules
// run. Rebuilding it with path.Join cost 5.3 GB of allocations in one dotCMS
// scan, so the joined value is memoized.
func TestMemEditorPathLookupsAreMemoized(t *testing.T) {
	editor := NewMemEditor("package main\n")
	editor.SetProgramName("prog")
	editor.SetFolderPath("a/b")
	editor.SetFileName("c.go")

	require.Equal(t, "/a/b/c.go", editor.GetFilePath())
	require.Equal(t, "/prog/a/b/c.go", editor.GetUrl())

	allocs := testing.AllocsPerRun(100, func() {
		if editor.GetFilePath() != "/a/b/c.go" {
			t.Fatalf("unexpected file path")
		}
		if editor.GetUrl() != "/prog/a/b/c.go" {
			t.Fatalf("unexpected url")
		}
	})
	require.Zero(t, allocs, "repeated path lookups allocated %v objects", allocs)
}

// The memo must never outlive its inputs.
func TestMemEditorPathLookupsFollowSetters(t *testing.T) {
	editor := NewMemEditor("package main\n")
	editor.SetProgramName("prog")
	editor.SetFolderPath("a")
	editor.SetFileName("c.go")
	require.Equal(t, "/a/c.go", editor.GetFilePath())
	require.Equal(t, "/prog/a/c.go", editor.GetUrl())

	editor.SetFileName("d.go")
	require.Equal(t, "/a/d.go", editor.GetFilePath(), "file name change must invalidate the memo")

	editor.SetFolderPath("b")
	require.Equal(t, "/b/d.go", editor.GetFilePath(), "folder change must invalidate the memo")

	editor.SetProgramName("other")
	require.Equal(t, "/other/b/d.go", editor.GetUrl(), "program change must invalidate the memo")

	editor.SetUrl("/x/y/e.go")
	require.Equal(t, "/other/x/y/e.go", editor.GetUrl(), "an explicit url must win over the memo")
	require.Equal(t, "/x/y/e.go", editor.GetFilePath(), "the file path must follow the explicit url")
}
