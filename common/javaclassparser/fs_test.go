package javaclassparser

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func testJavaJarPositiveDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	dir := filepath.Join(wd, "..", "sca", "testdata", "java_jar", "positive")
	abs, err := filepath.Abs(dir)
	require.NoError(t, err)
	require.DirExists(t, abs)
	return abs
}

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

func TestExpandedLocalFileSystem_ListArchiveAsDirectory(t *testing.T) {
	dir := testJavaJarPositiveDir(t)
	jarPath := filepath.Join(dir, "test.jar")

	fs := NewExpandedLocalFileSystem()

	info, err := fs.Stat(jarPath)
	require.NoError(t, err)
	require.True(t, info.IsDir(), "archive on disk should stat as directory")

	entries, err := fs.ReadDir(jarPath)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
}

func TestJarRecursiveParseEnabled(t *testing.T) {
	require.True(t, JarRecursiveParseEnabled(nil))
	trueVal := true
	require.True(t, JarRecursiveParseEnabled(&trueVal))
	falseVal := false
	require.False(t, JarRecursiveParseEnabled(&falseVal))
}

func TestJarRecursiveParseEnabledFromString(t *testing.T) {
	require.True(t, JarRecursiveParseEnabledFromString(""))
	require.True(t, JarRecursiveParseEnabledFromString("true"))
	require.False(t, JarRecursiveParseEnabledFromString("false"))
	require.True(t, JarRecursiveParseEnabledFromString("not-a-bool"))
}

func TestLocalFileSystemForJarRecursiveParse_Disabled(t *testing.T) {
	dir := testJavaJarPositiveDir(t)
	jarPath := filepath.Join(dir, "test.jar")

	fs := NewLocalFileSystemForJarRecursiveParse(false)

	info, err := fs.Stat(jarPath)
	require.NoError(t, err)
	require.False(t, info.IsDir(), "jar should remain an opaque file when recursive parse is disabled")

	_, err = fs.ReadDir(jarPath)
	require.Error(t, err)
}

func TestExpandedLocalFileSystem_DecompileClassInsideJar(t *testing.T) {
	dir := testJavaJarPositiveDir(t)
	jarPath := filepath.Join(dir, "test.jar")
	classInsideJar := filepath.Join(jarPath, "javax", "websocket", "ContainerProvider.class")

	fs := NewExpandedLocalFileSystem()
	content, err := fs.ReadFile(classInsideJar)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(content), "class"))
	require.False(t, strings.HasPrefix(string(content), "CAFE BABE"))
}

func TestExpandedLocalFileSystem_WarAndPar(t *testing.T) {
	dir := testJavaJarPositiveDir(t)
	fs := NewExpandedLocalFileSystem()

	for _, name := range []string{"test.war", "test.par"} {
		t.Run(name, func(t *testing.T) {
			archivePath := filepath.Join(dir, name)
			if _, err := os.Stat(archivePath); err != nil {
				t.Skipf("%s not present: %v", name, err)
			}
			info, err := fs.Stat(archivePath)
			require.NoError(t, err)
			require.True(t, info.IsDir(), "%s should stat as directory", name)

			entries, err := fs.ReadDir(archivePath)
			require.NoError(t, err)
			require.NotEmpty(t, entries)
		})
	}
}

func TestExpandedLocalFileSystem_NestedJarOnDisk(t *testing.T) {
	jarPath, cleanup := createTestJarWithNestedJar(t)
	defer cleanup()

	tempDir := filepath.Dir(jarPath)
	fs := NewExpandedLocalFileSystem()
	mainJarOnDisk := filepath.Join(tempDir, "main.jar")

	info, err := fs.Stat(mainJarOnDisk)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	nestedClassPath := filepath.Join(mainJarOnDisk, "lib", "nested.jar", "com", "example", "NestedClass.class")
	data, err := fs.ReadFile(nestedClassPath)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestExpandedLocalFileSystem_NestedJarInWar(t *testing.T) {
	dir := testJavaJarPositiveDir(t)
	warPath := filepath.Join(dir, "test.war")
	if _, err := os.Stat(warPath); err != nil {
		t.Skipf("test.war not present: %v", err)
	}

	fs := NewExpandedLocalFileSystem()
	nestedJarPath := filepath.Join(warPath, "WEB-INF", "lib", "commons-lang3-3.11.jar")

	info, err := fs.Stat(nestedJarPath)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	entries, err := fs.ReadDir(nestedJarPath)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	classPath := filepath.Join(nestedJarPath, "org", "apache", "commons", "lang3", "StringUtils.class")
	data, err := fs.ReadFile(classPath)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	require.NotEqual(t, byte(0xca), data[0], "class file should be decompiled, not raw CAFE BABE")
}

func TestExpandedLocalFileSystem_StringEscapeUtilsDecompile(t *testing.T) {
	dir := testJavaJarPositiveDir(t)
	warPath := filepath.Join(dir, "test.war")
	if _, err := os.Stat(warPath); err != nil {
		t.Skipf("test.war not present: %v", err)
	}

	fs := NewExpandedLocalFileSystem()
	classPath := filepath.Join(warPath, "WEB-INF", "lib", "commons-lang3-3.11.jar", "org", "apache", "commons", "lang3", "StringEscapeUtils.class")
	data, err := fs.ReadFile(classPath)
	require.NoError(t, err)
	s := string(data)
	require.NotContains(t, s, "new String[][")
	require.Contains(t, s, "new String[")
}

func TestDecompileClassBytes_DumpFailureReturnsStub(t *testing.T) {
	dir := testJavaJarPositiveDir(t)
	zipFS, err := filesys.NewZipFSFromLocal(filepath.Join(dir, "test.jar"))
	require.NoError(t, err)
	raw, err := zipFS.ReadFile("org/apache/tomcat/websocket/WsRemoteEndpointImplClient.class")
	require.NoError(t, err)

	out := decompileClassBytes("org/apache/tomcat/websocket/WsRemoteEndpointImplClient.class", raw)
	require.NotEqual(t, byte(0xca), out[0])
	require.Contains(t, string(out), "decompile dump failed")
}

func TestParseArchivePath(t *testing.T) {
	archivePath, internalPath, ok := parseArchivePath("C:/tmp/test.jar/com/example/Main.class")
	require.True(t, ok)
	require.Equal(t, "C:/tmp/test.jar", archivePath)
	require.Equal(t, "com/example/Main.class", internalPath)

	archivePath, internalPath, ok = parseArchivePath(`C:\tmp\test.jar\com\example\Main.class`)
	require.True(t, ok)
	require.Equal(t, "C:/tmp/test.jar", archivePath)
	require.Equal(t, "com/example/Main.class", internalPath)
}

