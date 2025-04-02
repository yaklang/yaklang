package java_decompiler

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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

func TestReadJarDirectory(t *testing.T) {
	// Create a temporary JAR file with nested JARs for testing
	tempJarPath, _, cleanup := createTestJarWithNested(t)
	defer cleanup()

	// Test listing the root directory of the main JAR
	dirPath := "."
	rootURL, err := CreateUrlFromString("javadec:///jar-aifix?jar=" + tempJarPath + "&dir=" + dirPath)
	require.NoError(t, err)

	rootParams := &ypb.RequestYakURLParams{
		Url:    rootURL,
		Method: "GET",
	}
	rootResp, err := NewJavaDecompilerAction().Get(rootParams)
	if err != nil {
		t.Fatal(err)
	}

	// Verify we can list the main JAR directory
	found := false
	for _, res := range rootResp.Resources {
		t.Logf("Path: %s", res.Path)
		if res.Path == "com/example" || res.Path == "com" {
			found = true
		}
	}
	require.True(t, found, "Should find 'com' or 'com/example' directory in main JAR")
}

func TestReadJarClass(t *testing.T) {
	// Create a temporary JAR file with nested JARs for testing
	tempJarPath, _, cleanup := createTestJarWithNested(t)
	defer cleanup()

	// Test accessing a class in the main JAR
	classPath := "com/example/MainClass.class"
	rootURL, err := CreateUrlFromString("javadec:///class-aifix?jar=" + tempJarPath + "&class=" + classPath)
	require.NoError(t, err)

	rootParams := &ypb.RequestYakURLParams{
		Url:    rootURL,
		Method: "GET",
	}
	rootResp, err := NewJavaDecompilerAction().Get(rootParams)
	if err != nil {
		t.Fatal(err)
	}

	// Verify we can access the class in the main JAR
	found := false
	for _, res := range rootResp.Resources {
		t.Logf("Resource: %s, Type: %s", res.Path, res.ResourceType)
		if res.ResourceType == "class" && strings.HasSuffix(res.Path, "MainClass.class") {
			found = true
		}
	}
	require.True(t, found, "Should find MainClass.class in main JAR")
}

func TestNestedJarAccess(t *testing.T) {
	// Create a temporary JAR file with nested JARs for testing
	tempJarPath, _, cleanup := createTestJarWithNested(t)
	defer cleanup()

	// First check if we can list the directory containing the nested JAR
	dirPath := "lib"
	rootURL, err := CreateUrlFromString("javadec:///jar-aifix?jar=" + tempJarPath + "&dir=" + dirPath)
	require.NoError(t, err)

	rootParams := &ypb.RequestYakURLParams{
		Url:    rootURL,
		Method: "GET",
	}
	rootResp, err := NewJavaDecompilerAction().Get(rootParams)
	require.NoError(t, err)

	// Verify we can see the nested JAR
	nestedJarFound := false
	for _, res := range rootResp.Resources {
		t.Logf("Path in lib dir: %s", res.Path)
		if strings.Contains(res.Path, "nested.jar") {
			nestedJarFound = true
		}
	}
	require.True(t, nestedJarFound, "Should find nested.jar in lib directory")

	// Create a copy of the nested JAR for testing
	action := NewJavaDecompilerAction()
	jarFS, err := action.getJarFS(tempJarPath)
	require.NoError(t, err)

	// Read the nested JAR content
	nestedJarContent, err := jarFS.ZipFS.ReadFile("lib/nested.jar")
	require.NoError(t, err)

	// Create temporary file for nested JAR
	nestedJarPath := filepath.Join(t.TempDir(), "nested.jar")
	err = os.WriteFile(nestedJarPath, nestedJarContent, 0644)
	require.NoError(t, err)

	// Now we can properly test nested JAR access using the extracted file
	nestedDirPath := "com/example"
	nestedURL, err := CreateUrlFromString("javadec:///jar-aifix?jar=" + nestedJarPath + "&dir=" + nestedDirPath)
	require.NoError(t, err)

	nestedParams := &ypb.RequestYakURLParams{
		Url:    nestedURL,
		Method: "GET",
	}
	nestedResp, err := action.Get(nestedParams)
	require.NoError(t, err)

	// Verify we can see content in the nested JAR
	classFound := false
	for _, res := range nestedResp.Resources {
		t.Logf("Path in nested JAR: %s, Type: %s", res.Path, res.ResourceType)
		if strings.Contains(res.Path, "NestedClass.class") {
			classFound = true
		}
	}
	require.True(t, classFound, "Should find NestedClass.class in nested JAR")
}

func TestDeeplyNestedJarAccess(t *testing.T) {
	// Create a temporary JAR file with nested JARs for testing
	tempJarPath, _, cleanup := createTestJarWithNested(t)
	defer cleanup()

	// Create a copy of the nested JAR for testing
	action := NewJavaDecompilerAction()
	jarFS, err := action.getJarFS(tempJarPath)
	require.NoError(t, err)

	// Read the nested JAR content
	nestedJarContent, err := jarFS.ZipFS.ReadFile("lib/nested.jar")
	require.NoError(t, err)

	// Create temporary file for nested JAR
	tempDir := t.TempDir()
	nestedJarPath := filepath.Join(tempDir, "nested.jar")
	err = os.WriteFile(nestedJarPath, nestedJarContent, 0644)
	require.NoError(t, err)

	// Now extract the inner JAR from nested JAR
	nestedJarFS, err := action.getJarFS(nestedJarPath)
	require.NoError(t, err)

	// Read the inner JAR content
	innerJarContent, err := nestedJarFS.ZipFS.ReadFile("lib/inner.jar")
	require.NoError(t, err)

	// Create temporary file for inner JAR
	innerJarPath := filepath.Join(tempDir, "inner.jar")
	err = os.WriteFile(innerJarPath, innerJarContent, 0644)
	require.NoError(t, err)

	// Now we can properly test inner JAR access
	innerDirPath := "com/example"
	innerURL, err := CreateUrlFromString("javadec:///jar-aifix?jar=" + innerJarPath + "&dir=" + innerDirPath)
	require.NoError(t, err)

	innerParams := &ypb.RequestYakURLParams{
		Url:    innerURL,
		Method: "GET",
	}
	innerResp, err := action.Get(innerParams)
	require.NoError(t, err)

	// Verify we can see content in the inner JAR
	classFound := false
	for _, res := range innerResp.Resources {
		t.Logf("Path in inner JAR: %s, Type: %s", res.Path, res.ResourceType)
		if strings.Contains(res.Path, "InnerClass.class") {
			classFound = true
		}
	}
	require.True(t, classFound, "Should find InnerClass.class in inner JAR")

	// Test reading a class from the inner JAR
	classPath := "com/example/InnerClass.class"
	classURL, err := CreateUrlFromString("javadec:///class-aifix?jar=" + innerJarPath + "&class=" + classPath)
	require.NoError(t, err)

	classParams := &ypb.RequestYakURLParams{
		Url:    classURL,
		Method: "GET",
	}
	classResp, err := action.Get(classParams)
	require.NoError(t, err)

	// Verify we can access the class in the inner JAR
	classContentFound := false
	for _, res := range classResp.Resources {
		t.Logf("Resource: %s, Type: %s", res.Path, res.ResourceType)
		if strings.Contains(res.Path, "InnerClass.class") {
			classContentFound = true
			// Check for extra data that should contain decompiled content
			for _, extra := range res.Extra {
				if extra.Key == "content" {
					t.Logf("Found class content, length: %d", len(extra.Value))
				}
			}
		}
	}
	require.True(t, classContentFound, "Should find InnerClass.class content in inner JAR")
}
