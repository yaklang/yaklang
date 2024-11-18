package tests

import (
	"embed"
	"os"
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
		//{
		//	"LambdaTest",
		//},
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
	}
	for _, testItem := range testCase {
		t.Run(testItem.name, func(t *testing.T) {
			t.Parallel()
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
		})
	}

}

//go:embed testfile
var javazip embed.FS

func TestDisCompilerJar(t *testing.T) {
	// javaclassparser.NewJarFSFromLocal()
	dir := os.TempDir()
	jar, err := javazip.ReadFile("testfile/test.jar")
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
