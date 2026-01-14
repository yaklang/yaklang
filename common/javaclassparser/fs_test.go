package javaclassparser

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// createTestJarWithNestedJar creates a temporary JAR file with a nested JAR for testing
func createTestJarWithNestedJar(t *testing.T) (string, func()) {
	t.Helper()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "jar-fs-test-*")
	require.NoError(t, err)

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	// Create nested JAR first
	nestedJarPath := filepath.Join(tempDir, "nested.jar")
	nestedJarFile, err := os.Create(nestedJarPath)
	require.NoError(t, err)
	defer nestedJarFile.Close()

	nestedJarWriter := zip.NewWriter(nestedJarFile)
	nestedClassWriter, err := nestedJarWriter.Create("com/example/NestedClass.class")
	require.NoError(t, err)
	nestedClassContent := []byte("fake class content")
	_, err = nestedClassWriter.Write(nestedClassContent)
	require.NoError(t, err)
	err = nestedJarWriter.Close()
	require.NoError(t, err)

	// Create main JAR with nested JAR inside
	mainJarPath := filepath.Join(tempDir, "main.jar")
	mainJarFile, err := os.Create(mainJarPath)
	require.NoError(t, err)
	defer mainJarFile.Close()

	mainJarWriter := zip.NewWriter(mainJarFile)

	// Add a class file to the main JAR
	mainClassWriter, err := mainJarWriter.Create("com/example/MainClass.class")
	require.NoError(t, err)
	mainClassContent := []byte("fake main class content")
	_, err = mainClassWriter.Write(mainClassContent)
	require.NoError(t, err)

	// Read the nested JAR file to include it in the main JAR
	nestedJarBytes, err := os.ReadFile(nestedJarPath)
	require.NoError(t, err)

	// Add the nested JAR to the main JAR
	nestedJarInMainWriter, err := mainJarWriter.Create("lib/nested.jar")
	require.NoError(t, err)
	_, err = nestedJarInMainWriter.Write(nestedJarBytes)
	require.NoError(t, err)

	// Close the main JAR
	err = mainJarWriter.Close()
	require.NoError(t, err)

	return mainJarPath, cleanup
}

func TestJarFS_RecursiveParse_Enabled(t *testing.T) {
	jarPath, cleanup := createTestJarWithNestedJar(t)
	defer cleanup()

	// Create JarFS with recursive parse enabled (default)
	zipFS, err := filesys.NewZipFSFromLocal(jarPath)
	require.NoError(t, err)
	jarFS := NewJarFSWithOptions(zipFS, true)

	t.Run("should read files from main jar", func(t *testing.T) {
		data, err := jarFS.ReadFile("com/example/MainClass.class")
		require.NoError(t, err)
		assert.NotEmpty(t, data)
	})

	t.Run("should read files from nested jar when recursive parse is enabled", func(t *testing.T) {
		// Try to read a file from the nested jar
		data, err := jarFS.ReadFile("lib/nested.jar/com/example/NestedClass.class")
		require.NoError(t, err, "should be able to read from nested jar when recursive parse is enabled")
		assert.NotEmpty(t, data)
	})

	t.Run("should list nested jar directory when recursive parse is enabled", func(t *testing.T) {
		entries, err := jarFS.ReadDir("lib/nested.jar")
		require.NoError(t, err, "should be able to list nested jar directory when recursive parse is enabled")
		assert.Greater(t, len(entries), 0, "should have entries in nested jar")
	})

	t.Run("should stat nested jar path when recursive parse is enabled", func(t *testing.T) {
		info, err := jarFS.Stat("lib/nested.jar/com/example/NestedClass.class")
		require.NoError(t, err, "should be able to stat nested jar file when recursive parse is enabled")
		assert.NotNil(t, info)
		assert.False(t, info.IsDir(), "class file should not be a directory")
	})
}

func TestJarFS_RecursiveParse_Disabled(t *testing.T) {
	jarPath, cleanup := createTestJarWithNestedJar(t)
	defer cleanup()

	// Create JarFS with recursive parse disabled
	zipFS, err := filesys.NewZipFSFromLocal(jarPath)
	require.NoError(t, err)
	jarFS := NewJarFSWithOptions(zipFS, false)

	t.Run("should read files from main jar", func(t *testing.T) {
		data, err := jarFS.ReadFile("com/example/MainClass.class")
		require.NoError(t, err)
		assert.NotEmpty(t, data)
	})

	t.Run("should not read files from nested jar when recursive parse is disabled", func(t *testing.T) {
		// Try to read a file from the nested jar - should fail
		_, err := jarFS.ReadFile("lib/nested.jar/com/example/NestedClass.class")
		assert.Error(t, err, "should not be able to read from nested jar when recursive parse is disabled")
		assert.Contains(t, err.Error(), "not exist", "error should indicate file not found")
	})

	t.Run("should not list nested jar directory when recursive parse is disabled", func(t *testing.T) {
		_, err := jarFS.ReadDir("lib/nested.jar")
		assert.Error(t, err, "should not be able to list nested jar directory when recursive parse is disabled")
	})

	t.Run("should not stat nested jar path when recursive parse is disabled", func(t *testing.T) {
		_, err := jarFS.Stat("lib/nested.jar/com/example/NestedClass.class")
		assert.Error(t, err, "should not be able to stat nested jar file when recursive parse is disabled")
	})

	t.Run("should still stat nested jar as file when recursive parse is disabled", func(t *testing.T) {
		// The nested jar file itself should still be accessible
		info, err := jarFS.Stat("lib/nested.jar")
		require.NoError(t, err, "should be able to stat nested jar file itself")
		assert.NotNil(t, info)
		// When recursive parse is disabled, nested jars are treated as directories
		assert.True(t, info.IsDir(), "nested jar should be marked as directory when recursive parse is disabled")
	})
}

func TestJarFS_DefaultBehavior(t *testing.T) {
	jarPath, cleanup := createTestJarWithNestedJar(t)
	defer cleanup()

	// Create JarFS with default constructor (should enable recursive parse)
	zipFS, err := filesys.NewZipFSFromLocal(jarPath)
	require.NoError(t, err)
	jarFS := NewJarFS(zipFS)

	t.Run("default should enable recursive parse", func(t *testing.T) {
		// Try to read a file from the nested jar - should succeed with default
		data, err := jarFS.ReadFile("lib/nested.jar/com/example/NestedClass.class")
		require.NoError(t, err, "default behavior should enable recursive parse")
		assert.NotEmpty(t, data)
	})
}

