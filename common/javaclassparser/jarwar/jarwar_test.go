package jarwar

import (
	_ "embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

//go:embed testdata/plexus-cipher-2.0.jar
var plexusCipherJar []byte

func TestNew(t *testing.T) {
	zfs, err := filesys.NewZipFSFromString(string(plexusCipherJar))
	assert.NoError(t, err)
	jfs := javaclassparser.NewJarFS(zfs)
	ins, err := NewFromJarFS(jfs)
	assert.NoError(t, err)
	fmt.Println(ins.GetStructDump())
	// 获取临时目录路径
	tempDir := t.TempDir()

	// 测试导出到临时目录
	err = ins.DumpToLocalFileSystem(tempDir)
	assert.NoError(t, err)

	// 验证导出的文件结构
	var files []string
	err = filesys.Recursive(tempDir, filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		files = append(files, s)
		return nil
	}))
	assert.NoError(t, err)
	assert.NotEmpty(t, files)

	// 验证 MANIFEST.MF 文件存在且内容正确
	manifestContent, err := os.ReadFile(filepath.Join(tempDir, "META-INF", "MANIFEST.MF"))
	assert.NoError(t, err)
	assert.Contains(t, string(manifestContent), "Manifest-Version")

	// 验证反编译的 java 文件
	javaFiles := lo.Filter(files, func(f string, _ int) bool {
		raw, _ := os.ReadFile(f)
		fmt.Println(string(raw))
		return strings.HasSuffix(f, ".java")
	})
	assert.NotEmpty(t, javaFiles)

	// 验证反编译失败的文件记录
	assert.NotNil(t, ins.failedDecompiledFiles)
}
