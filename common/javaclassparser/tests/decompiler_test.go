package tests

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/javaclassparser/classes"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestDecompiler(t *testing.T) {
	testCase := []struct {
		name string
	}{
		{"LongTest"},
		{"LogicalOperationMini"},
		{"SelfOp"},
		{"ContinuousAssign"},
		{"TryCatch1"},
		{"VarFold"},
		{"SuperTest"},
		{"FinalTest"},
		{
			"SynchronizedTest",
		},
		{
			"LambdaTest",
		},
		{
			"IfTest",
		},
		{
			"InterfaceTest",
		},
		{
			"TryCatch",
		},
		{
			name: "LogicalOperation",
		},
		{
			name: "TernaryExpressionTest",
		},
		{
			name: "SwitchTest",
		},
		{
			name: "StaticCodeBlockTest",
		},
		//{
		//	name: "AnnotationTest",
		//},
	}
	for _, testItem := range testCase {
		t.Run(testItem.name, func(t *testing.T) {
			// for i := 0; i < 100; i++ {
			//t.Parallel()
			classRaw, err := classes.FS.ReadFile(testItem.name + ".class")
			if err != nil {
				t.Fatal(err)
			}
			sourceCode, err := classes.FS.ReadFile(testItem.name + ".java")
			if err != nil {
				t.Fatal(err)
			}
			ins, err := javaclassparser.Parse(classRaw)
			if err != nil {
				t.Fatal(err)
			}
			source, err := ins.Dump()
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, string(sourceCode), source)
			// }
		})
	}

}

func TestDisCompilerJar(t *testing.T) {
	// javaclassparser.NewJarFSFromLocal()
	dir := os.TempDir()
	jar, err := classes.FS.ReadFile("test.jar")
	require.NoError(t, err)

	jarPath := dir + "/test.jar"
	err = os.WriteFile(jarPath, jar, 0644)
	require.NoError(t, err)
	// test jar filesystem
	jarFs, err := javaclassparser.NewJarFSFromLocal(jarPath)
	require.NoError(t, err)

	t.Run("test jar walk", func(t *testing.T) {
		fileList := make([]string, 0)
		filesys.Recursive(
			".",
			filesys.WithFileSystem(jarFs),
			filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
				if !strings.HasSuffix(pathname, ".class") {
					return nil
				}
				if isDir {
					return nil
				}
				fileList = append(fileList, pathname)

				data, err := jarFs.ReadFile(pathname)
				if err != nil {
					require.NoErrorf(t, err, "read file %s failed: %v", pathname, err)
				}
				log.Info(string(data))
				return nil
			}),
		)
		require.True(t, len(fileList) > 0)
	})
}

func TestSyntax(t *testing.T) {
	testCase := []struct {
		name string
	}{
		{
			"VarArgs",
		},
		{
			"SwitchScopeTest",
		},
	}
	for _, testItem := range testCase {
		t.Run(testItem.name, func(t *testing.T) {
			t.Parallel()
			fileName := filepath.Join("syntax_test", testItem.name)
			classRaw, err := classes.FS.ReadFile(fileName + ".class")
			if err != nil {
				t.Fatal(err)
			}
			sourceCode, err := classes.FS.ReadFile(fileName + ".java")
			if err != nil {
				t.Fatal(err)
			}
			ins, err := javaclassparser.Parse(classRaw)
			if err != nil {
				t.Fatal(err)
			}
			source, err := ins.Dump()
			if err != nil {
				t.Fatal(err)
			}
			println(source)
			assert.Equal(t, string(sourceCode), source)
		})
	}

}

func TestZipWithJar(t *testing.T) {
	// 创建包含 JAR 的 ZIP 文件
	dir := os.TempDir()
	jar, err := classes.FS.ReadFile("test.jar")
	require.NoError(t, err)

	// 创建 ZIP 文件，包含 test.jar 和 readme.txt
	var zipBuf bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuf)

	// 添加 readme.txt
	readmeEntry, err := zipWriter.Create("readme.txt")
	require.NoError(t, err)
	_, err = readmeEntry.Write([]byte("This is a test ZIP containing a JAR file."))
	require.NoError(t, err)

	// 添加 test.jar 到 lib/ 目录
	jarEntry, err := zipWriter.Create("lib/test.jar")
	require.NoError(t, err)
	_, err = io.Copy(jarEntry, bytes.NewReader(jar))
	require.NoError(t, err)

	err = zipWriter.Close()
	require.NoError(t, err)

	zipPath := dir + "/test-with-jar.zip"
	err = os.WriteFile(zipPath, zipBuf.Bytes(), 0644)
	require.NoError(t, err)

	zipFS, err := filesys.NewZipFSFromLocal(zipPath)
	require.NoError(t, err)
	expandedFS := javaclassparser.NewExpandedZipFS(zipFS, zipFS)

	t.Run("test zip with nested jar Recursive", func(t *testing.T) {
		// 测试 Recursive 遍历能够展开 JAR
		fileList := make([]string, 0)
		filesys.Recursive(
			".",
			filesys.WithFileSystem(expandedFS),
			filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
				if !isDir {
					fileList = append(fileList, pathname)
					if strings.Contains(pathname, ".jar/") {
						data, err := expandedFS.ReadFile(pathname)
						if err != nil {
							log.Warnf("read file %s failed: %v", pathname, err)
							return nil
						}
						require.NotEmpty(t, data, "file %s should have content", pathname)
					}
				}
				return nil
			}),
		)
		require.Greater(t, len(fileList), 0, "should find files")
		log.Infof("file list: %v", fileList)
		// 检查是否存在 readme.txt 文件（ZIP 根目录）
		hasReadme := false
		for _, file := range fileList {
			if file == "readme.txt" || strings.HasSuffix(file, "readme.txt") {
				hasReadme = true
				break
			}
		}
		require.True(t, hasReadme, "should find readme.txt in ZIP root: fileList=%v", fileList)

		// 检查是否存在 Main.java 文件（嵌套 JAR 中）
		hasMainClass := false
		for _, file := range fileList {
			if strings.Contains(file, "Main.java") || strings.HasSuffix(file, "Main.java") {
				hasMainClass = true
				break
			}
		}
		require.True(t, hasMainClass, "should find Main.java in nested jar: fileList=%v", fileList)

		// 直接使用 expandedFS 验证 Main.java 是否存在（因为 Recursive 可能无法遍历嵌套 JAR）
		mainClassPath := "lib/test.jar/com/java/main/Main.java"
		data, err := expandedFS.ReadFile(mainClassPath)
		log.Info(data)
		require.NoError(t, err, "should be able to read Main.java from nested jar: %s", mainClassPath)
		require.NotEmpty(t, data, "Main.java should have content")
		log.Infof("successfully read Main.java from nested jar: %s", mainClassPath)
	})
}
