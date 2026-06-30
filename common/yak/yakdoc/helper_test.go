package yakdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldSkipDocWalkDir(t *testing.T) {
	require.True(t, shouldSkipDocWalkDir("test"))
	require.True(t, shouldSkipDocWalkDir("tests"))
	require.True(t, shouldSkipDocWalkDir("testdata"))
	require.True(t, shouldSkipDocWalkDir(".git"))
	require.False(t, shouldSkipDocWalkDir("yakdoc"))
}

func TestDocParseFileFilter(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fixture.go"), []byte("//go:build ignore\n\npackage main\n"), 0o644))

	filter := docParseFileFilter(dir)
	info, err := os.Stat(filepath.Join(dir, "main.go"))
	require.NoError(t, err)
	require.True(t, filter(info))

	info, err = os.Stat(filepath.Join(dir, "main_test.go"))
	require.NoError(t, err)
	require.False(t, filter(info))

	info, err = os.Stat(filepath.Join(dir, "fixture.go"))
	require.NoError(t, err)
	require.False(t, filter(info))
}

func TestGetProjectAstPackages(t *testing.T) {
	t.SkipNow()

	pkgs, _, err := GetProjectAstPackages()
	if err != nil {
		t.Fatal(err)
	}

	for path, pkg := range pkgs {
		fmt.Printf("%s: %s\n", path, pkg.Name)
	}
}

func TestGetProjectAstPackagesSkipsSyntaxTestDirs(t *testing.T) {
	pkgs, _, err := GetProjectAstPackages()
	require.NoError(t, err)

	for path := range pkgs {
		require.NotContains(t, path, "/go2ssa/test/")
		require.NotContains(t, path, "/test/code")
	}
}
