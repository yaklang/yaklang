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

// createMinimalClassFile returns a valid .class file for testing
// We read an existing class file as a template
func createMinimalClassFile(className string) []byte {
	// Read the basic1.class file from the javaclassparser tests
	classFile := filepath.Join("..", "..", "..", "javaclassparser", "tests", "basic1.class")
	data, err := os.ReadFile(classFile)
	if err != nil {
		// If file not found, return a minimal valid class file structure
		// This is a fallback that should work for basic testing
		return []byte{
			0xCA, 0xFE, 0xBA, 0xBE, // Magic
			0x00, 0x00, // Minor version
			0x00, 0x37, // Major version (Java 11)
			0x00, 0x02, // Constant pool count
			0x01, 0x00, 0x10, 0x6A, 0x61, 0x76, 0x61, 0x2F, 0x6C, 0x61, 0x6E, 0x67, 0x2F, 0x4F, 0x62, 0x6A, 0x65, 0x63, 0x74, // CONSTANT_Utf8: "java/lang/Object"
			0x07, 0x00, 0x01, // CONSTANT_Class: java/lang/Object
			0x00, 0x21, // Access flags: ACC_PUBLIC | ACC_SUPER
			0x00, 0x02, // This class
			0x00, 0x01, // Super class
			0x00, 0x00, // Interfaces count
			0x00, 0x00, // Fields count
			0x00, 0x00, // Methods count
			0x00, 0x00, // Attributes count
		}
	}
	return data
}

func createTestZipWithMultipleJars(t *testing.T) (string, map[string][]byte, func()) {
	t.Helper()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "zip-jar-test-*")
	require.NoError(t, err)

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	// Map to store content for verification
	zipContent := make(map[string][]byte)

	// Create first JAR file
	jar1Path := filepath.Join(tempDir, "app1.jar")
	jar1File, err := os.Create(jar1Path)
	require.NoError(t, err)
	defer jar1File.Close()

	jar1Writer := zip.NewWriter(jar1File)

	// Add a class file to the first JAR
	// Create a minimal valid .class file (CAFEBABE magic, version, constant pool, etc.)
	jar1ClassContent := createMinimalClassFile("com/app1/App1Class")
	zipContent["app1.jar/com/app1/App1Class.class"] = jar1ClassContent

	jar1ClassWriter, err := jar1Writer.Create("com/app1/App1Class.class")
	require.NoError(t, err)
	_, err = jar1ClassWriter.Write(jar1ClassContent)
	require.NoError(t, err)

	err = jar1Writer.Close()
	require.NoError(t, err)

	// Create second JAR file
	jar2Path := filepath.Join(tempDir, "app2.jar")
	jar2File, err := os.Create(jar2Path)
	require.NoError(t, err)
	defer jar2File.Close()

	jar2Writer := zip.NewWriter(jar2File)

	// Add a class file to the second JAR
	jar2ClassContent := createMinimalClassFile("com/app2/App2Class")
	zipContent["app2.jar/com/app2/App2Class.class"] = jar2ClassContent

	jar2ClassWriter, err := jar2Writer.Create("com/app2/App2Class.class")
	require.NoError(t, err)
	_, err = jar2ClassWriter.Write(jar2ClassContent)
	require.NoError(t, err)

	err = jar2Writer.Close()
	require.NoError(t, err)

	// Create third JAR file
	jar3Path := filepath.Join(tempDir, "libs/utils.jar")
	err = os.MkdirAll(filepath.Dir(jar3Path), 0755)
	require.NoError(t, err)

	jar3File, err := os.Create(jar3Path)
	require.NoError(t, err)
	defer jar3File.Close()

	jar3Writer := zip.NewWriter(jar3File)

	// Add a class file to the third JAR
	jar3ClassContent := createMinimalClassFile("com/utils/UtilsClass")
	zipContent["libs/utils.jar/com/utils/UtilsClass.class"] = jar3ClassContent

	jar3ClassWriter, err := jar3Writer.Create("com/utils/UtilsClass.class")
	require.NoError(t, err)
	_, err = jar3ClassWriter.Write(jar3ClassContent)
	require.NoError(t, err)

	err = jar3Writer.Close()
	require.NoError(t, err)

	// Create the main ZIP file
	zipPath := filepath.Join(tempDir, "test.zip")
	zipFile, err := os.Create(zipPath)
	require.NoError(t, err)
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)

	// Read and add first JAR to ZIP
	jar1Bytes, err := os.ReadFile(jar1Path)
	require.NoError(t, err)
	jar1InZipWriter, err := zipWriter.Create("app1.jar")
	require.NoError(t, err)
	_, err = jar1InZipWriter.Write(jar1Bytes)
	require.NoError(t, err)

	// Read and add second JAR to ZIP
	jar2Bytes, err := os.ReadFile(jar2Path)
	require.NoError(t, err)
	jar2InZipWriter, err := zipWriter.Create("app2.jar")
	require.NoError(t, err)
	_, err = jar2InZipWriter.Write(jar2Bytes)
	require.NoError(t, err)

	// Read and add third JAR to ZIP (in libs subdirectory)
	jar3Bytes, err := os.ReadFile(jar3Path)
	require.NoError(t, err)
	jar3InZipWriter, err := zipWriter.Create("libs/utils.jar")
	require.NoError(t, err)
	_, err = jar3InZipWriter.Write(jar3Bytes)
	require.NoError(t, err)

	// Add a regular text file to the ZIP
	textContent := []byte("This is a regular text file in the ZIP")
	textWriter, err := zipWriter.Create("readme.txt")
	require.NoError(t, err)
	_, err = textWriter.Write(textContent)
	require.NoError(t, err)

	err = zipWriter.Close()
	require.NoError(t, err)

	return zipPath, zipContent, cleanup
}

func TestReadZipWithMultipleJars(t *testing.T) {
	// Create a temporary ZIP file with multiple JARs for testing
	tempZipPath, _, cleanup := createTestZipWithMultipleJars(t)
	defer cleanup()

	// Test listing the root directory of the ZIP
	dirPath := "."
	rootURL, err := CreateUrlFromString("javadec:///jar-aifix?jar=" + tempZipPath + "&dir=" + dirPath)
	require.NoError(t, err)

	rootParams := &ypb.RequestYakURLParams{
		Url:    rootURL,
		Method: "GET",
	}
	rootResp, err := NewJavaDecompilerAction().Get(rootParams)
	require.NoError(t, err)

	// Verify we can see the JAR files in the ZIP
	jar1Found := false
	jar2Found := false
	libsFound := false
	for _, res := range rootResp.Resources {
		t.Logf("Path: %s, Type: %s", res.Path, res.ResourceType)
		if strings.Contains(res.Path, "app1.jar") {
			jar1Found = true
		}
		if strings.Contains(res.Path, "app2.jar") {
			jar2Found = true
		}
		if strings.Contains(res.Path, "libs") {
			libsFound = true
		}
	}
	require.True(t, jar1Found, "Should find app1.jar in ZIP")
	require.True(t, jar2Found, "Should find app2.jar in ZIP")
	require.True(t, libsFound, "Should find libs directory in ZIP")
}

func TestAccessJarInZip(t *testing.T) {
	// Create a temporary ZIP file with multiple JARs for testing
	tempZipPath, _, cleanup := createTestZipWithMultipleJars(t)
	defer cleanup()

	// Test accessing a class in the first JAR within ZIP
	// Format: jar-path/class-path
	classPath := "app1.jar/com/app1/App1Class.class"
	classURL, err := CreateUrlFromString("javadec:///class-aifix?jar=" + tempZipPath + "&class=" + classPath)
	require.NoError(t, err)

	classParams := &ypb.RequestYakURLParams{
		Url:    classURL,
		Method: "GET",
	}
	classResp, err := NewJavaDecompilerAction().Get(classParams)
	require.NoError(t, err)

	// Verify we can access the class in the first JAR
	classFound := false
	for _, res := range classResp.Resources {
		t.Logf("Resource: %s, Type: %s", res.Path, res.ResourceType)
		if res.ResourceType == "class" && strings.Contains(res.Path, "App1Class.class") {
			classFound = true
		}
	}
	require.True(t, classFound, "Should find App1Class.class in app1.jar within ZIP")
}

func TestAccessMultipleJarsInZip(t *testing.T) {
	// Create a temporary ZIP file with multiple JARs for testing
	tempZipPath, _, cleanup := createTestZipWithMultipleJars(t)
	defer cleanup()

	action := NewJavaDecompilerAction()

	// Test accessing class from first JAR
	class1Path := "app1.jar/com/app1/App1Class.class"
	class1URL, err := CreateUrlFromString("javadec:///class-aifix?jar=" + tempZipPath + "&class=" + class1Path)
	require.NoError(t, err)

	class1Params := &ypb.RequestYakURLParams{
		Url:    class1URL,
		Method: "GET",
	}
	class1Resp, err := action.Get(class1Params)
	require.NoError(t, err)

	class1Found := false
	for _, res := range class1Resp.Resources {
		if strings.Contains(res.Path, "App1Class.class") {
			class1Found = true
		}
	}
	require.True(t, class1Found, "Should find App1Class.class in app1.jar")

	// Test accessing class from second JAR
	class2Path := "app2.jar/com/app2/App2Class.class"
	class2URL, err := CreateUrlFromString("javadec:///class-aifix?jar=" + tempZipPath + "&class=" + class2Path)
	require.NoError(t, err)

	class2Params := &ypb.RequestYakURLParams{
		Url:    class2URL,
		Method: "GET",
	}
	class2Resp, err := action.Get(class2Params)
	require.NoError(t, err)

	class2Found := false
	for _, res := range class2Resp.Resources {
		if strings.Contains(res.Path, "App2Class.class") {
			class2Found = true
		}
	}
	require.True(t, class2Found, "Should find App2Class.class in app2.jar")

	// Test accessing class from third JAR in subdirectory
	class3Path := "libs/utils.jar/com/utils/UtilsClass.class"
	class3URL, err := CreateUrlFromString("javadec:///class-aifix?jar=" + tempZipPath + "&class=" + class3Path)
	require.NoError(t, err)

	class3Params := &ypb.RequestYakURLParams{
		Url:    class3URL,
		Method: "GET",
	}
	class3Resp, err := action.Get(class3Params)
	require.NoError(t, err)

	class3Found := false
	for _, res := range class3Resp.Resources {
		if strings.Contains(res.Path, "UtilsClass.class") {
			class3Found = true
		}
	}
	require.True(t, class3Found, "Should find UtilsClass.class in libs/utils.jar")
}

func TestListJarDirectoryInZip(t *testing.T) {
	// Create a temporary ZIP file with multiple JARs for testing
	tempZipPath, _, cleanup := createTestZipWithMultipleJars(t)
	defer cleanup()

	// Test listing directory inside a JAR within ZIP
	// Format: jar-path/dir-path
	dirPath := "app1.jar/com/app1"
	dirURL, err := CreateUrlFromString("javadec:///jar-aifix?jar=" + tempZipPath + "&dir=" + dirPath)
	require.NoError(t, err)

	dirParams := &ypb.RequestYakURLParams{
		Url:    dirURL,
		Method: "GET",
	}
	dirResp, err := NewJavaDecompilerAction().Get(dirParams)
	require.NoError(t, err)

	// Verify we can list the directory inside the JAR
	classFound := false
	for _, res := range dirResp.Resources {
		t.Logf("Path in JAR directory: %s, Type: %s", res.Path, res.ResourceType)
		if strings.Contains(res.Path, "App1Class.class") {
			classFound = true
		}
	}
	require.True(t, classFound, "Should find App1Class.class in app1.jar/com/app1 directory")
}
