package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStatDirectoryWithSubdirs 测试 Stat 方法可以正确识别目录
func TestStatDirectoryWithSubdirs(t *testing.T) {
	// 测试根目录
	t.Run("root directory", func(t *testing.T) {
		info, err := FS.Stat(".")
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.True(t, info.IsDir(), "root should be a directory")
	})

	// 测试空路径（应该视为根目录）
	t.Run("empty path as root", func(t *testing.T) {
		info, err := FS.Stat("")
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.True(t, info.IsDir(), "empty path should be treated as root")
	})

	// 测试文件
	t.Run("file stat", func(t *testing.T) {
		info, err := FS.Stat("1.txt")
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.False(t, info.IsDir(), "1.txt should be a file")
		assert.Greater(t, info.Size(), int64(0), "file should have size")
	})

	// 测试不存在的路径
	t.Run("nonexistent path", func(t *testing.T) {
		_, err := FS.Stat("nonexistent/path/to/file")
		assert.Error(t, err)
	})

	// 测试带尾部斜杠的路径
	t.Run("path with trailing slash", func(t *testing.T) {
		info, err := FS.Stat("./")
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.True(t, info.IsDir())
	})
}
