package filesys

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestUnifiedFS_Base(t *testing.T) {
	temp := t.TempDir()
	path := filepath.Join(temp, "a", "b", "c.txt")
	err := os.MkdirAll(filepath.Dir(path), 0755)
	require.NoError(t, err)
	content := uuid.NewString()
	err = os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	lf := NewRelLocalFs(temp)
	uf := NewUnifiedFS(lf, WithUnifiedFsSeparator('$'))
	t.Run("TestUnifiedFS_Join", func(t *testing.T) {
		dir1 := uf.Join("a", "b", "c")
		require.Equal(t, "a$b$c", dir1)

		dir2 := uf.Join("a", "..", "d")
		require.Equal(t, "d", dir2)

		dir3 := uf.Join(".", "a", "b", "c")
		require.Equal(t, "a$b$c", dir3)
	})

	t.Run("TestUnifiedFS_PathSplit", func(t *testing.T) {
		dir, file := uf.PathSplit("a$b$c")
		require.Equal(t, "a$b", dir)
		require.Equal(t, "c", file)
	})

	t.Run("TestUnifiedFS_Base", func(t *testing.T) {
		base := uf.Base("a$b$c")
		require.Equal(t, "c", base)
	})

	t.Run("TestUnifiedFS_Ext", func(t *testing.T) {
		ext := uf.Ext("a$b$c.txt")
		require.Equal(t, ".txt", ext)
	})

	t.Run("TestUnifiedFS_IsAbs", func(t *testing.T) {
		abs := uf.IsAbs("$a$b$c.txt")
		require.True(t, abs)
	})
}

func TestUnifiedFs_File_Operate(t *testing.T) {
	temp := t.TempDir()
	path := filepath.Join(temp, "a", "b", "c.txt")
	err := os.MkdirAll(filepath.Dir(path), 0755)
	require.NoError(t, err)
	content := uuid.NewString()
	err = os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	lf := NewRelLocalFs(temp)
	uf := NewUnifiedFS(lf, WithUnifiedFsSeparator('$'))
	unifiedPath := "a$b$c.txt"
	t.Run("TestUnifiedFS_OpenFile", func(t *testing.T) {
		_, err := uf.OpenFile(unifiedPath, os.O_RDONLY, 0644)
		require.NoError(t, err)
	})

	t.Run("TestUnifiedFS_Exists", func(t *testing.T) {
		exist, err := uf.Exists(unifiedPath)
		require.NoError(t, err)
		require.True(t, exist)
	})

	t.Run("TestUnifiedFS_Mkdir", func(t *testing.T) {
		p := "aaa$bbb"
		err := uf.MkdirAll(p, 0755)
		require.NoError(t, err)
		exist, err := uf.Exists(p)
		require.NoError(t, err)
		require.True(t, exist)
	})

	t.Run("TestUnifiedFS_Stat", func(t *testing.T) {
		info, err := uf.Stat(unifiedPath)
		require.NoError(t, err)
		require.Equal(t, int64(len(content)), info.Size())
	})
}

func TestUnifiedFs_ClassToJavaExtMap(t *testing.T) {
	temp := t.TempDir()
	lf := NewRelLocalFs(temp)
	uf := NewUnifiedFS(
		lf,
		WithUnifiedFsExtMap(".class", ".java"),
	)

	t.Run("TestUnifiedFS_ConvertToRealPath", func(t *testing.T) {
		require.Equal(t, uf.convertToRealPathWithOp("test.class", ReadOperation), "test.java")
		require.Equal(t, uf.convertToRealPathWithOp("test.java", WriteOperation), "test.class")
	})

	t.Run("TestWriteClassFile", func(t *testing.T) {
		unifiedPath := "Example.java"
		expectedRealPath := filepath.Join(temp, "Example.class")

		content := uuid.NewString()
		err := uf.WriteFile(unifiedPath, []byte(content), 0644)
		require.NoError(t, err)

		realContent, err := os.ReadFile(expectedRealPath)
		require.NoError(t, err)
		require.Equal(t, []byte(content), realContent)
	})

	t.Run("TestReadJavaFileThroughUnifiedFS", func(t *testing.T) {
		unifiedPath := "Example.java"
		expectedRealPath := filepath.Join(temp, "Example.class")
		err := os.WriteFile(expectedRealPath, []byte("test content"), 0644)
		require.NoError(t, err)
		data, err := uf.ReadFile(unifiedPath)
		require.NoError(t, err)
		require.Equal(t, []byte("test content"), data)
	})

	t.Run("TestRenameClassFile", func(t *testing.T) {
		unifiedOld := "OldExample.java"
		unifiedNew := "RenamedExample.java"
		expectedOldPath := filepath.Join(temp, "OldExample.class")
		expectedNewPath := filepath.Join(temp, "RenamedExample.class")
		// 创建测试文件
		err := os.WriteFile(expectedOldPath, []byte("rename test"), 0644)
		require.NoError(t, err)
		// 执行重命名
		err = uf.Rename(unifiedOld, unifiedNew)
		require.NoError(t, err)
		// 验证新文件存在
		_, err = os.Stat(expectedNewPath)
		require.NoError(t, err)
		// 验证旧文件已删除
		_, err = os.Stat(expectedOldPath)
		require.Error(t, err)
	})

	t.Run("TestDeleteClassFile", func(t *testing.T) {
		unifiedPath := "ToDelete.java"
		expectedRealPath := filepath.Join(temp, "ToDelete.class")

		// 创建测试文件
		err := os.WriteFile(expectedRealPath, []byte("delete test"), 0644)
		require.NoError(t, err)

		// 执行删除
		err = uf.Delete(unifiedPath)
		require.NoError(t, err)

		// 验证文件已删除
		_, err = os.Stat(expectedRealPath)
		require.Error(t, err)
	})

	t.Run("TestFileNotBeAffected", func(t *testing.T) {
		unifiedPath := "README.md"
		expectedRealPath := filepath.Join(temp, "README.md")

		err := os.WriteFile(expectedRealPath, []byte("markdown content"), 0644)
		require.NoError(t, err)

		data, err := uf.ReadFile(unifiedPath)
		require.NoError(t, err)
		require.Equal(t, []byte("markdown content"), data)
	})

	t.Run("TestExtNoAffected", func(t *testing.T) {
		ret := uf.Ext("aaa.java")
		require.Equal(t, ".java", ret)
	})
}
