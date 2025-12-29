package java_decompiler

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func createTestJarWithNested(t *testing.T) (string, map[string][]byte) {
	t.Helper()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "jar-parser-test-*")
	require.NoError(t, err)

	// cleanup := func() {
	// 	os.RemoveAll(tempDir)
	// }

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

	return mainJarPath, jarContent
}

func TestReadJarDirectory(t *testing.T) {
	// Create a temporary JAR file with nested JARs for testing
	tempJarPath, _ := createTestJarWithNested(t)

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
	tempJarPath, _ := createTestJarWithNested(t)

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
	tempJarPath, _ := createTestJarWithNested(t)

	tempDir := t.TempDir()
	var nestedJarPath string
	var actions []*Action

	t.Cleanup(func() {
		for _, action := range actions {
			action.ClearCache()
		}
		// Force GC twice to ensure all references are released (Windows file handles
		// may require multiple GC cycles to be fully released)
		runtime.GC()
		runtime.GC()
		time.Sleep(200 * time.Millisecond)
	})

	t.Run("list nested jar", func(t *testing.T) {
		dirPath := "lib"
		rootURL, err := CreateUrlFromString("javadec:///jar-aifix?jar=" + tempJarPath + "&dir=" + dirPath)
		require.NoError(t, err)

		rootParams := &ypb.RequestYakURLParams{
			Url:    rootURL,
			Method: "GET",
		}
		rootAction := NewJavaDecompilerAction()
		actions = append(actions, rootAction)
		rootResp, err := rootAction.Get(rootParams)
		require.NoError(t, err)

		nestedJarFound := false
		for _, res := range rootResp.Resources {
			t.Logf("Path in lib dir: %s", res.Path)
			if strings.Contains(res.Path, "nested.jar") {
				nestedJarFound = true
			}
		}
		require.True(t, nestedJarFound, "Should find nested.jar in lib directory")
	})

	t.Run("extract nested jar", func(t *testing.T) {
		mainAction := NewJavaDecompilerAction()
		actions = append(actions, mainAction)

		jarFS, err := mainAction.getJarFS(tempJarPath)
		require.NoError(t, err)

		nestedJarContent, err := jarFS.ZipFS.ReadFile("lib/nested.jar")
		require.NoError(t, err)

		nestedJarPath = filepath.Join(tempDir, "nested.jar")
		err = os.WriteFile(nestedJarPath, nestedJarContent, 0644)
		require.NoError(t, err)
	})

	t.Run("access nested jar", func(t *testing.T) {
		nestedAction := NewJavaDecompilerAction()
		actions = append(actions, nestedAction)

		nestedDirPath := "com/example"
		nestedURL, err := CreateUrlFromString("javadec:///jar-aifix?jar=" + nestedJarPath + "&dir=" + nestedDirPath)
		require.NoError(t, err)

		nestedParams := &ypb.RequestYakURLParams{
			Url:    nestedURL,
			Method: "GET",
		}
		nestedResp, err := nestedAction.Get(nestedParams)
		require.NoError(t, err)

		classFound := false
		for _, res := range nestedResp.Resources {
			t.Logf("Path in nested JAR: %s, Type: %s", res.Path, res.ResourceType)
			if strings.Contains(res.Path, "NestedClass.class") {
				classFound = true
			}
		}
		require.True(t, classFound, "Should find NestedClass.class in nested JAR")
	})
}

func TestDeeplyNestedJarAccess(t *testing.T) {
	// Create a temporary JAR file with nested JARs for testing
	tempJarPath, _ := createTestJarWithNested(t)

	tempDir := t.TempDir()
	var nestedJarPath string
	var innerJarPath string
	var actions []*Action

	t.Cleanup(func() {
		for _, action := range actions {
			action.ClearCache()
		}
		// Force GC twice to ensure all references are released (Windows file handles
		// may require multiple GC cycles to be fully released)
		runtime.GC()
		runtime.GC()
		time.Sleep(200 * time.Millisecond)
	})

	t.Run("extract nested jar", func(t *testing.T) {
		mainAction := NewJavaDecompilerAction()
		actions = append(actions, mainAction)

		jarFS, err := mainAction.getJarFS(tempJarPath)
		require.NoError(t, err)

		nestedJarContent, err := jarFS.ZipFS.ReadFile("lib/nested.jar")
		require.NoError(t, err)

		nestedJarPath = filepath.Join(tempDir, "nested.jar")
		err = os.WriteFile(nestedJarPath, nestedJarContent, 0644)
		require.NoError(t, err)
	})

	t.Run("extract inner jar", func(t *testing.T) {
		nestedAction := NewJavaDecompilerAction()
		actions = append(actions, nestedAction)

		nestedJarFS, err := nestedAction.getJarFS(nestedJarPath)
		require.NoError(t, err)

		innerJarContent, err := nestedJarFS.ZipFS.ReadFile("lib/inner.jar")
		require.NoError(t, err)

		innerJarPath = filepath.Join(tempDir, "inner.jar")
		err = os.WriteFile(innerJarPath, innerJarContent, 0644)
		require.NoError(t, err)
	})

	t.Run("access inner jar", func(t *testing.T) {
		innerAction := NewJavaDecompilerAction()
		actions = append(actions, innerAction)

		innerDirPath := "com/example"
		innerURL, err := CreateUrlFromString("javadec:///jar-aifix?jar=" + innerJarPath + "&dir=" + innerDirPath)
		require.NoError(t, err)

		innerParams := &ypb.RequestYakURLParams{
			Url:    innerURL,
			Method: "GET",
		}
		innerResp, err := innerAction.Get(innerParams)
		require.NoError(t, err)

		classFound := false
		for _, res := range innerResp.Resources {
			t.Logf("Path in inner JAR: %s, Type: %s", res.Path, res.ResourceType)
			if strings.Contains(res.Path, "InnerClass.class") {
				classFound = true
			}
		}
		require.True(t, classFound, "Should find InnerClass.class in inner JAR")
	})

	t.Run("read class from inner jar", func(t *testing.T) {
		innerAction := NewJavaDecompilerAction()
		actions = append(actions, innerAction)

		classPath := "com/example/InnerClass.class"
		classURL, err := CreateUrlFromString("javadec:///class-aifix?jar=" + innerJarPath + "&class=" + classPath)
		require.NoError(t, err)

		classParams := &ypb.RequestYakURLParams{
			Url:    classURL,
			Method: "GET",
		}
		classResp, err := innerAction.Get(classParams)
		require.NoError(t, err)

		classContentFound := false
		for _, res := range classResp.Resources {
			t.Logf("Resource: %s, Type: %s", res.Path, res.ResourceType)
			if strings.Contains(res.Path, "InnerClass.class") {
				classContentFound = true
				for _, extra := range res.Extra {
					if extra.Key == "content" {
						t.Logf("Found class content, length: %d", len(extra.Value))
					}
				}
			}
		}
		require.True(t, classContentFound, "Should find InnerClass.class content in inner JAR")
	})
}
