package test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	"os"

	"github.com/stretchr/testify/assert"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var flag string

func init() {
	s := "dGhpcyBpcyBnZW4gZW1iZWQgdGVzdCBmaWxl"
	f, _ := codec.DecodeBase64(s)
	flag = string(f)
}
func TestFs(t *testing.T) {
	content, err := FS.ReadFile("1.txt")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, string(flag), string(content))
	p, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	exeContent, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	strFlag := "this is a test flag string"
	if !bytes.Contains(exeContent, []byte(strFlag)) {
		t.Fatal(errors.New("string flag should be in the executable file"))
	}
	if bytes.Contains(exeContent, []byte(flag)) {
		t.Fatal(errors.New("flag should not be in the executable file"))
	}
}

func TestCache(t *testing.T) {
	cachedFs, err := gzip_embed.NewPreprocessingEmbed(&resourceFS, "static.tar.gz", true)
	if err != nil {
		t.Fatal(err)
	}
	notcachedFs, err := gzip_embed.NewPreprocessingEmbed(&resourceFS, "static.tar.gz", false)
	if err != nil {
		t.Fatal(err)
	}
	content, err := cachedFs.ReadFile("1.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != flag {
		t.Fatal(errors.New("read file by cached fs failed"))
	}
	content, err = notcachedFs.ReadFile("1.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != flag {
		t.Fatal(errors.New("read file by not cached fs failed"))
	}
	calcDuration := func(fs *gzip_embed.PreprocessingEmbed) int64 {
		start := time.Now()
		for i := 0; i < 100; i++ {
			_, err := fs.ReadFile("1.txt")
			if err != nil {
				t.Fatal(err)
			}
		}
		return time.Since(start).Nanoseconds()
	}
	cachedDu := calcDuration(cachedFs)
	notcachedDu := calcDuration(notcachedFs)
	fmt.Printf("cached fs duration: %d, not cached fs duration: %d\n", cachedDu, notcachedDu)
	if cachedDu*10 >= notcachedDu {
		t.Fatal(errors.New("cached fs should be at least 10 times faster than not cached fs"))
	}
}

func TestGetHash(t *testing.T) {
	// 测试 GetHash 功能
	hash1, err := FS.GetHash()
	if err != nil {
		t.Fatal(err)
	}
	assert.NotEmpty(t, hash1, "hash should not be empty")
	assert.Equal(t, 64, len(hash1), "SHA256 hash should be 64 characters")

	// 再次调用应该返回缓存的哈希值
	hash2, err := FS.GetHash()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, hash1, hash2, "cached hash should be the same")

	// 测试 InvalidateHash
	FS.InvalidateHash()
	hash3, err := FS.GetHash()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, hash1, hash3, "hash should be the same after invalidation and recalculation")
}

func TestFileSystemInterface(t *testing.T) {
	// 验证 PreprocessingEmbed 实现了 fi.FileSystem 接口
	var _ fi.FileSystem = FS

	// 测试 Open 方法
	file, err := FS.Open("1.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	// 测试 Stat 方法
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "1.txt", info.Name())
	assert.False(t, info.IsDir())

	// 测试 ReadDir 方法
	entries, err := FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	assert.NotEmpty(t, entries, "should have at least one entry")

	// 测试路径方法
	assert.Equal(t, '/', FS.GetSeparators())
	assert.Equal(t, "a/b/c", FS.Join("a", "b", "c"))
	assert.Equal(t, "file.txt", FS.Base("path/to/file.txt"))
	assert.Equal(t, ".txt", FS.Ext("file.txt"))

	// 测试 Exists 方法
	exists, err := FS.Exists("1.txt")
	assert.NoError(t, err)
	assert.True(t, exists)

	exists, err = FS.Exists("nonexistent.txt")
	assert.Error(t, err)
	assert.False(t, exists)

	// 测试 ExtraInfo 方法
	info2 := FS.ExtraInfo("")
	assert.Equal(t, "gzip_embed", info2["type"])
	assert.Equal(t, "static.tar.gz", info2["source_file"])
}

func TestReadOnlyOperations(t *testing.T) {
	// 测试只读操作应该返回错误
	err := FS.Rename("old", "new")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")

	err = FS.WriteFile("test.txt", []byte("test"), 0644)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")

	err = FS.Delete("test.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")

	err = FS.MkdirAll("test", 0755)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestStatDirectory(t *testing.T) {
	// 测试 Stat 方法可以处理目录

	// 测试根目录
	info, err := FS.Stat(".")
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.True(t, info.IsDir(), "root should be a directory")
	assert.Equal(t, ".", info.Name())

	// 测试空字符串（也应该是根目录）
	info, err = FS.Stat("")
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.True(t, info.IsDir(), "empty path should be treated as root directory")

	// 测试文件
	info, err = FS.Stat("1.txt")
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.False(t, info.IsDir(), "1.txt should be a file")
	assert.Equal(t, "1.txt", info.Name())

	// 测试不存在的路径
	_, err = FS.Stat("nonexistent/path")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")

	// 如果有子目录，测试子目录
	// 首先检查是否有子目录
	entries, err := FS.ReadDir(".")
	assert.NoError(t, err)

	var hasSubDir bool
	var subDirName string
	for _, entry := range entries {
		if entry.IsDir() {
			hasSubDir = true
			subDirName = entry.Name()
			break
		}
	}

	if hasSubDir {
		t.Logf("测试子目录: %s", subDirName)
		info, err = FS.Stat(subDirName)
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.True(t, info.IsDir(), "%s should be a directory", subDirName)
		assert.Equal(t, subDirName, info.Name())
	} else {
		t.Log("没有找到子目录，跳过子目录测试")
	}
}
