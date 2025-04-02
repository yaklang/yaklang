package jar

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"archive/zip"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJarParser(t *testing.T) {
	// Create a temporary JAR file with nested JARs for testing
	tempJarPath, _, cleanup := createTestJarWithNested(t)
	defer cleanup()

	// Create a new jar parser
	parser, err := NewJarParser(tempJarPath)
	require.NoError(t, err)
	require.NotNil(t, parser)

	// Test basic functionality
	t.Run("TestGetJarFS", func(t *testing.T) {
		fs := parser.GetJarFS()
		require.NotNil(t, fs)
	})

	t.Run("TestListDirectory", func(t *testing.T) {
		entries, err := parser.ListDirectory(".")
		require.NoError(t, err)
		assert.True(t, len(entries) > 0, "JAR should have at least one entry")
	})

	t.Run("TestGetDirectoryContents", func(t *testing.T) {
		contents, err := parser.GetDirectoryContents(".")
		require.NoError(t, err)
		assert.True(t, len(contents) > 0, "JAR should have at least one content item")

		// Verify the content structure
		for _, item := range contents {
			assert.Contains(t, item, "name")
			assert.Contains(t, item, "path")
			assert.Contains(t, item, "size")
			assert.Contains(t, item, "isDirectory")
			assert.Contains(t, item, "lastModified")
		}
	})

	t.Run("TestFindJavaClasses", func(t *testing.T) {
		classes, err := parser.FindJavaClasses()
		require.NoError(t, err)
		assert.True(t, len(classes) > 0, "JAR should have at least one Java class")

		// Verify that all paths end with .class
		for _, class := range classes {
			assert.True(t, strings.HasSuffix(class, ".class"))
		}
	})

	t.Run("TestNestedJarHandling", func(t *testing.T) {
		// Try to access the nested JAR
		nestedJarPath := "lib/nested.jar"

		// Check that we can list the contents of the nested JAR
		entries, err := parser.ListDirectory(nestedJarPath + "/")
		require.NoError(t, err)
		assert.True(t, len(entries) > 0, "Nested JAR should have at least one entry")

		// Try listing a directory in the nested JAR
		entries, err = parser.ListDirectory(nestedJarPath + "/com")
		require.NoError(t, err)
		assert.True(t, len(entries) > 0, "Directory in nested JAR should have entries")

		// Try accessing a class in the nested JAR
		decompiled, err := parser.DecompileClass(nestedJarPath + "/com/example/NestedClass.class")
		require.NoError(t, err)
		assert.True(t, len(decompiled) > 0, "Should be able to decompile a class from the nested JAR")
	})

	t.Run("TestMultiLevelNestedJar", func(t *testing.T) {
		// Try to access the multi-level nested JAR
		nestedJarPath := "lib/nested.jar/lib/inner.jar"

		// Check that we can list the contents of the multi-level nested JAR
		entries, err := parser.ListDirectory(nestedJarPath + "/")
		require.NoError(t, err)
		assert.True(t, len(entries) > 0, "Multi-level nested JAR should have entries")

		// Try accessing a class in the multi-level nested JAR
		decompiled, err := parser.DecompileClass(nestedJarPath + "/com/example/InnerClass.class")
		require.NoError(t, err)
		assert.True(t, len(decompiled) > 0, "Should be able to decompile a class from the multi-level nested JAR")
	})

	t.Run("TestExportDecompiledJar", func(t *testing.T) {
		// Export the JAR
		buf, err := parser.ExportDecompiledJar()
		require.NoError(t, err)
		assert.True(t, buf.Len() > 0, "Exported JAR should have content")

		// Save it to a temporary file for debugging if needed
		tmpFile := filepath.Join(os.TempDir(), "exported-jar-test.zip")
		err = os.WriteFile(tmpFile, buf.Bytes(), 0644)
		require.NoError(t, err)
		t.Logf("Exported JAR saved to: %s", tmpFile)
	})

	// t.Run("TestFindJavaClassesWithNestedJars", func(t *testing.T) {
	// 	// Test with includeNestedJars=true to search in nested JARs
	// 	allClasses, err := parser.FindJavaClasses(true)
	// 	require.NoError(t, err)

	// 	// We should find classes in nested JARs
	// 	foundNestedClass := false
	// 	foundInnerClass := false

	// 	for _, class := range allClasses {
	// 		if strings.Contains(class, "nested.jar/com/example/NestedClass.class") {
	// 			foundNestedClass = true
	// 		}
	// 		if strings.Contains(class, "nested.jar/lib/inner.jar/com/example/InnerClass.class") {
	// 			foundInnerClass = true
	// 		}
	// 	}

	// 	assert.True(t, foundNestedClass, "Should find classes in first-level nested JAR")
	// 	assert.True(t, foundInnerClass, "Should find classes in second-level nested JAR")
	// })
}

// createTestJarWithNested creates temporary JAR files with nested structure for testing
// Returns:
// - path to the main JAR
// - map of the nested JAR's content (for verification)
// - cleanup function to remove temporary files
func createTestJarWithNested(t *testing.T) (string, map[string][]byte, func()) {
	t.Helper()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "jar-parser-test-*")
	require.NoError(t, err)

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	// Create temp JAR file paths
	mainJarPath := filepath.Join(tempDir, "test.jar")

	// Map to store content for verification
	jarContent := make(map[string][]byte)

	// Create inner-level JAR (deepest level)
	innerJarPath := filepath.Join(tempDir, "inner.jar")
	innerJarFile, err := os.Create(innerJarPath)
	require.NoError(t, err)
	defer innerJarFile.Close()

	innerJarWriter := zip.NewWriter(innerJarFile)

	// Add a class file to the inner JAR
	innerClassContent := []byte(
		"package com.example;\n\n" +
			"public class InnerClass {\n" +
			"    public static void main(String[] args) {\n" +
			"        System.out.println(\"Hello from InnerClass\");\n" +
			"    }\n" +
			"}\n")
	jarContent["inner.jar/com/example/InnerClass.class"] = innerClassContent

	// Create the class file in the inner JAR
	innerClassWriter, err := innerJarWriter.Create("com/example/InnerClass.class")
	require.NoError(t, err)
	_, err = innerClassWriter.Write(innerClassContent)
	require.NoError(t, err)

	// Create another file in the inner JAR
	innerTextWriter, err := innerJarWriter.Create("inner-file.txt")
	require.NoError(t, err)
	innerTextContent := []byte("This is a text file inside the inner JAR")
	_, err = innerTextWriter.Write(innerTextContent)
	require.NoError(t, err)

	// Close the inner JAR
	err = innerJarWriter.Close()
	require.NoError(t, err)

	// Create middle-level (nested) JAR
	nestedJarPath := filepath.Join(tempDir, "nested.jar")
	nestedJarFile, err := os.Create(nestedJarPath)
	require.NoError(t, err)
	defer nestedJarFile.Close()

	nestedJarWriter := zip.NewWriter(nestedJarFile)

	// Add a class file to the nested JAR
	nestedClassContent := []byte(
		"package com.example;\n\n" +
			"public class NestedClass {\n" +
			"    public static void main(String[] args) {\n" +
			"        System.out.println(\"Hello from NestedClass\");\n" +
			"    }\n" +
			"}\n")
	jarContent["nested.jar/com/example/NestedClass.class"] = nestedClassContent

	// Create the class file in the nested JAR
	nestedClassWriter, err := nestedJarWriter.Create("com/example/NestedClass.class")
	require.NoError(t, err)
	_, err = nestedClassWriter.Write(nestedClassContent)
	require.NoError(t, err)

	// Read the inner JAR file to include it in the nested JAR
	innerJarBytes, err := os.ReadFile(innerJarPath)
	require.NoError(t, err)

	// Add the inner JAR to the nested JAR
	innerJarInNestedWriter, err := nestedJarWriter.Create("lib/inner.jar")
	require.NoError(t, err)
	_, err = innerJarInNestedWriter.Write(innerJarBytes)
	require.NoError(t, err)

	// Close the nested JAR
	err = nestedJarWriter.Close()
	require.NoError(t, err)

	// Create the main JAR
	mainJarFile, err := os.Create(mainJarPath)
	require.NoError(t, err)
	defer mainJarFile.Close()

	mainJarWriter := zip.NewWriter(mainJarFile)

	// Add a class file to the main JAR
	mainClassContent := []byte(
		"package com.example;\n\n" +
			"public class MainClass {\n" +
			"    public static void main(String[] args) {\n" +
			"        System.out.println(\"Hello from MainClass\");\n" +
			"    }\n" +
			"}\n")
	jarContent["com/example/MainClass.class"] = mainClassContent

	// Create the main class file
	mainClassWriter, err := mainJarWriter.Create("com/example/MainClass.class")
	require.NoError(t, err)
	_, err = mainClassWriter.Write(mainClassContent)
	require.NoError(t, err)

	// Create an inner class in the main JAR
	innerClassInMainContent := []byte(
		"package com.example;\n\n" +
			"public class MainClass$InnerClass {\n" +
			"    public void hello() {\n" +
			"        System.out.println(\"Hello from inner class\");\n" +
			"    }\n" +
			"}\n")
	jarContent["com/example/MainClass$InnerClass.class"] = innerClassInMainContent

	innerClassInMainWriter, err := mainJarWriter.Create("com/example/MainClass$InnerClass.class")
	require.NoError(t, err)
	_, err = innerClassInMainWriter.Write(innerClassInMainContent)
	require.NoError(t, err)

	// Add a simple manifest file
	manifestWriter, err := mainJarWriter.Create("META-INF/MANIFEST.MF")
	require.NoError(t, err)
	manifestContent := []byte("Manifest-Version: 1.0\nCreated-By: JAR Parser Test\nMain-Class: com.example.MainClass\n")
	_, err = manifestWriter.Write(manifestContent)
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

	return mainJarPath, jarContent, cleanup
}

func TestJarParserFromBytes(t *testing.T) {
	// Create a temporary JAR file with nested JARs for testing
	tempJarPath, _, cleanup := createTestJarWithNested(t)
	defer cleanup()

	// Read JAR file into memory
	jarContent, err := os.ReadFile(tempJarPath)
	require.NoError(t, err)

	// Create a new jar parser from bytes
	parser, err := NewJarParserFromBytes(jarContent)
	require.NoError(t, err)
	require.NotNil(t, parser)

	// Test that we can list the contents
	entries, err := parser.ListDirectory(".")
	require.NoError(t, err)
	assert.True(t, len(entries) > 0, "JAR from bytes should have at least one entry")
}

func TestInnerClassDetection(t *testing.T) {
	// Create a temporary JAR file with nested JARs for testing
	tempJarPath, _, cleanup := createTestJarWithNested(t)
	defer cleanup()

	// Create a new jar parser
	parser, err := NewJarParser(tempJarPath)
	require.NoError(t, err)

	// Get all classes
	classes, err := parser.FindJavaClasses()
	require.NoError(t, err)

	// Find an outer class (if any)
	var outerClass string
	for _, class := range classes {
		if !strings.Contains(filepath.Base(class), "$") {
			outerClass = class
			break
		}
	}

	assert.NotEmpty(t, outerClass, "Should find at least one outer class")

	// Test finding inner classes
	innerClasses, err := parser.FindInnerClasses(outerClass)
	require.NoError(t, err)

	// Our test JAR should have at least one inner class
	assert.True(t, len(innerClasses) > 0, "Should find at least one inner class")

	// We know we created MainClass$InnerClass, so check for it
	var foundInnerClass bool
	for _, innerClass := range innerClasses {
		if strings.Contains(innerClass, "MainClass$InnerClass.class") {
			foundInnerClass = true
			break
		}
	}
	assert.True(t, foundInnerClass, "Should find MainClass$InnerClass specifically")

	// Log the results
	t.Logf("Outer class: %s", outerClass)
	t.Logf("Inner classes: %v", innerClasses)
}

func TestJarManifest(t *testing.T) {
	// Create a temporary JAR file with nested JARs for testing
	tempJarPath, _, cleanup := createTestJarWithNested(t)
	defer cleanup()

	// Create a new jar parser
	parser, err := NewJarParser(tempJarPath)
	require.NoError(t, err)

	// Test getting the manifest
	manifest, err := parser.GetJarManifest()
	require.NoError(t, err, "Test JAR should have a manifest")

	// Check the manifest entries we created
	assert.Equal(t, "1.0", manifest["Manifest-Version"], "Manifest-Version should be 1.0")
	assert.Equal(t, "JAR Parser Test", manifest["Created-By"], "Created-By should be JAR Parser Test")
	assert.Equal(t, "com.example.MainClass", manifest["Main-Class"], "Main-Class should be com.example.MainClass")

	// Print manifest entries
	t.Logf("Manifest entries:")
	for key, value := range manifest {
		t.Logf("%s: %s", key, value)
	}
}

func TestNestedJarPath(t *testing.T) {
	parser := &JarParser{}

	tests := []struct {
		path          string
		wantJarPath   string
		wantInnerPath string
		wantErr       bool
	}{
		{
			path:          "lib/sample.jar/com/example/Main.class",
			wantJarPath:   "lib/sample.jar",
			wantInnerPath: "com/example/Main.class",
			wantErr:       false,
		},
		{
			path:          "sample.jar",
			wantJarPath:   "sample.jar",
			wantInnerPath: "",
			wantErr:       false,
		},
		{
			path:          "path/without/jar",
			wantJarPath:   "",
			wantInnerPath: "",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			jarPath, innerPath, err := parser.ParseNestedJarPath(tt.path)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantJarPath, jarPath)
				assert.Equal(t, tt.wantInnerPath, innerPath)
			}
		})
	}
}

func TestNestedJarFunctionality(t *testing.T) {
	// Create a temporary JAR file with nested JARs for testing
	tempJarPath, _, cleanup := createTestJarWithNested(t)
	defer cleanup()

	// Create a new jar parser
	parser, err := NewJarParser(tempJarPath)
	require.NoError(t, err)

	// Test cases for different levels of JAR nesting
	testCases := []struct {
		name         string
		path         string
		expectError  bool
		errorMessage string
	}{
		{
			name:        "Regular directory",
			path:        "com",
			expectError: false,
		},
		{
			name:        "Regular class",
			path:        "com/example/MainClass.class",
			expectError: false,
		},
		{
			name:        "First-level nested JAR directory",
			path:        "lib/nested.jar/com",
			expectError: false,
		},
		{
			name:        "First-level nested JAR class",
			path:        "lib/nested.jar/com/example/NestedClass.class",
			expectError: false,
		},
		{
			name:        "Second-level nested JAR directory",
			path:        "lib/nested.jar/lib/inner.jar/com",
			expectError: false,
		},
		{
			name:        "Second-level nested JAR class",
			path:        "lib/nested.jar/lib/inner.jar/com/example/InnerClass.class",
			expectError: false,
		},
		{
			name:         "Nonexistent nested JAR",
			path:         "lib/nonexistent.jar/com/example/Main.class",
			expectError:  true,
			errorMessage: "failed to read nested jar",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// For directory paths, test ListDirectory
			if !strings.HasSuffix(tc.path, ".class") {
				entries, err := parser.ListDirectory(tc.path)

				if tc.expectError {
					assert.Error(t, err)
					if tc.errorMessage != "" {
						assert.Contains(t, err.Error(), tc.errorMessage)
					}
				} else {
					require.NoError(t, err, "ListDirectory should work for %s", tc.path)
					assert.True(t, len(entries) > 0, "ListDirectory should return entries for %s", tc.path)
				}
			} else {
				// For class paths, test DecompileClass
				decompiled, err := parser.DecompileClass(tc.path)

				if tc.expectError {
					assert.Error(t, err)
					if tc.errorMessage != "" {
						assert.Contains(t, err.Error(), tc.errorMessage)
					}
				} else {
					require.NoError(t, err, "DecompileClass should work for %s", tc.path)
					assert.True(t, len(decompiled) > 0, "Decompiled class should have content for %s", tc.path)

					// Verify it looks like a Java class
					assert.True(t,
						bytes.Contains(decompiled, []byte("class ")) ||
							bytes.Contains(decompiled, []byte("interface ")) ||
							bytes.Contains(decompiled, []byte("public class")),
						"Decompiled content should look like Java code")
				}
			}
		})
	}
}

// TestNestedJarPathHandling tests the updated ParseNestedJarPath function with multiple levels
func TestNestedJarPathHandling(t *testing.T) {
	parser := &JarParser{}

	tests := []struct {
		name          string
		path          string
		wantJarPath   string
		wantInnerPath string
		wantErr       bool
	}{
		{
			name:          "Single nested JAR",
			path:          "lib/sample.jar/com/example/Main.class",
			wantJarPath:   "lib/sample.jar",
			wantInnerPath: "com/example/Main.class",
			wantErr:       false,
		},
		{
			name:          "Multiple nested JARs are not directly handled",
			path:          "lib/outer.jar/lib/inner.jar/com/example/Main.class",
			wantJarPath:   "lib/outer.jar",
			wantInnerPath: "lib/inner.jar/com/example/Main.class",
			wantErr:       false,
		},
		{
			name:          "Directory path in nested JAR",
			path:          "lib/sample.jar/com/example/",
			wantJarPath:   "lib/sample.jar",
			wantInnerPath: "com/example/",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jarPath, innerPath, err := parser.ParseNestedJarPath(tt.path)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantJarPath, jarPath)
				assert.Equal(t, tt.wantInnerPath, innerPath)
			}
		})
	}
}

// TestMultiLevelNestedJarPaths tests the parseMultiLevelJarPath function with multiple levels of nesting
func TestMultiLevelNestedJarPaths(t *testing.T) {
	parser := &JarParser{}

	tests := []struct {
		name          string
		path          string
		wantJarPaths  []string
		wantFinalPath string
		wantErr       bool
	}{
		{
			name:          "Single nested JAR",
			path:          "lib/sample.jar/com/example/Main.class",
			wantJarPaths:  []string{"lib/sample.jar"},
			wantFinalPath: "com/example/Main.class",
			wantErr:       false,
		},
		{
			name:          "Double nested JAR",
			path:          "lib/outer.jar/libs/inner.jar/com/example/Main.class",
			wantJarPaths:  []string{"lib/outer.jar", "libs/inner.jar"},
			wantFinalPath: "com/example/Main.class",
			wantErr:       false,
		},
		{
			name:          "Triple nested JAR",
			path:          "lib/level1.jar/data/level2.jar/vendor/level3.jar/com/example/Main.class",
			wantJarPaths:  []string{"lib/level1.jar", "data/level2.jar", "vendor/level3.jar"},
			wantFinalPath: "com/example/Main.class",
			wantErr:       false,
		},
		{
			name:          "Just a JAR file without internal path",
			path:          "lib/sample.jar",
			wantJarPaths:  []string{"lib/sample.jar"},
			wantFinalPath: "",
			wantErr:       false,
		},
		{
			name:          "Regular path (no JAR)",
			path:          "com/example/Main.class",
			wantJarPaths:  nil,
			wantFinalPath: "com/example/Main.class",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jarPaths, finalPath, err := parser.parseMultiLevelJarPath(tt.path)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantJarPaths, jarPaths, "JAR paths should match")
				assert.Equal(t, tt.wantFinalPath, finalPath, "Final path should match")
			}
		})
	}
}

// TestMultiLevelJarListDirectory tests that ListDirectory works with multiple nested JARs
func TestMultiLevelJarListDirectory(t *testing.T) {
	// Create a temporary JAR file with nested JARs for testing
	tempJarPath, _, cleanup := createTestJarWithNested(t)
	defer cleanup()

	// Create a new jar parser
	parser, err := NewJarParser(tempJarPath)
	require.NoError(t, err)

	// Try to list directories at different nesting levels
	testCases := []struct {
		name        string
		path        string
		expectError bool
		fileCount   int // expected number of files/directories
	}{
		{
			name:        "Root directory",
			path:        ".",
			expectError: false,
			fileCount:   4, // com/, lib/, META-INF/, (these are our known directories)
		},
		{
			name:        "First-level directory",
			path:        "com",
			expectError: false,
			fileCount:   1, // example/
		},
		{
			name:        "Class directory",
			path:        "com/example",
			expectError: false,
			fileCount:   2, // MainClass.class, MainClass$InnerClass.class
		},
		{
			name:        "First-level nested JAR",
			path:        "lib/nested.jar",
			expectError: false,
			fileCount:   2, // com/, lib/
		},
		{
			name:        "Directory in first-level nested JAR",
			path:        "lib/nested.jar/com/example",
			expectError: false,
			fileCount:   1, // NestedClass.class
		},
		{
			name:        "Second-level nested JAR",
			path:        "lib/nested.jar/lib/inner.jar",
			expectError: false,
			fileCount:   2, // com/, inner-file.txt
		},
		{
			name:        "Directory in second-level nested JAR",
			path:        "lib/nested.jar/lib/inner.jar/com/example",
			expectError: false,
			fileCount:   1, // InnerClass.class
		},
		{
			name:        "Multi-level nested JAR (nonexistent)",
			path:        "lib/outer.jar/libs/inner.jar/com/example",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			entries, err := parser.ListDirectory(tc.path)
			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to read nested jar")
			} else {
				require.NoError(t, err)
				if tc.fileCount > 0 {
					assert.Equal(t, tc.fileCount, len(entries), "Path %s should have %d entries", tc.path, tc.fileCount)
				} else {
					assert.True(t, len(entries) > 0, "Path %s should have at least one entry", tc.path)
				}

				// Log what we found for debugging
				fileNames := make([]string, 0, len(entries))
				for _, entry := range entries {
					fileNames = append(fileNames, entry.Name())
				}
				t.Logf("Files found in %s: %v", tc.path, fileNames)
			}
		})
	}
}
