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
	uf := ConvertToUnifiedFs(lf)

	t.Run("TestUnifiedFS_Join", func(t *testing.T) {
		t.Log(string(lf.GetSeparators()))
		dir := uf.Join("a", "b", "c")
		require.Equal(t, "a/b/c", dir)
	})

	t.Run("TestUnifiedFS_PathSplit", func(t *testing.T) {
		dir, file := uf.PathSplit("a/b/c")
		require.Equal(t, "a/b", dir)
		require.Equal(t, "c", file)
	})

	t.Run("TestUnifiedFS_Base", func(t *testing.T) {
		base := uf.Base("a/b/c")
		require.Equal(t, "c", base)
	})

	t.Run("TestUnifiedFS_Ext", func(t *testing.T) {
		ext := uf.Ext("a/b/c.txt")
		require.Equal(t, ".txt", ext)
	})

	t.Run("TestUnifiedFS_IsAbs", func(t *testing.T) {
		abs := uf.IsAbs("/a/b/c.txt")
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
	uf := ConvertToUnifiedFs(lf)
	unifiedPath := "a/b/c.txt"
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
		p := "aaa/bbb"
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
