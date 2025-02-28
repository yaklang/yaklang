package filesys

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"io/fs"
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
		info, err := uf.OpenFile(unifiedPath, os.O_RDONLY, 0644)
		require.NoError(t, err)
		fi, err := info.Stat()
		require.NoError(t, err)
		require.Equal(t, int64(len(content)), fi.Size())
		require.Equal(t, "c.txt", fi.Name())
	})

	t.Run("TestUnifiedFS_Exists", func(t *testing.T) {
		fileName := "a$a$c.txt"
		exist, _ := uf.Exists(fileName)
		require.False(t, exist)

		data := []byte(uuid.NewString())
		uf.MkdirAll(fileName, 0755)
		uf.WriteFile(fileName, data, 0755)
		exist, err := uf.Exists(fileName)
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
	innerFs := NewRelLocalFs(temp)
	outerFs := NewUnifiedFS(
		innerFs,
		WithUnifiedFsExtMap(".class", ".java"),
	)

	t.Run("TestWriteClassFile", func(t *testing.T) {
		unifiedPath := "Example.java"
		expectedRealPath := filepath.Join(temp, "Example.class")

		content := uuid.NewString()
		err := outerFs.WriteFile(unifiedPath, []byte(content), 0644)
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
		data, err := outerFs.ReadFile(unifiedPath)
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
		err = outerFs.Rename(unifiedOld, unifiedNew)
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
		err = outerFs.Delete(unifiedPath)
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

		data, err := outerFs.ReadFile(unifiedPath)
		require.NoError(t, err)
		require.Equal(t, []byte("markdown content"), data)
	})

	t.Run("TestExtNoAffected", func(t *testing.T) {
		ret := outerFs.Ext("aaa.java")
		require.Equal(t, ".java", ret)
	})

	t.Run("TestVirtualExtNoAffected", func(t *testing.T) {
		unifiedPath := "testAAA.java"
		outerFs.WriteFile(unifiedPath, []byte("test content"), 0644)
		dir, err := outerFs.ReadDir(".")
		require.NoError(t, err)
		check := false
		for _, entry := range dir {
			if entry.Name() == "testAAA.java" {
				check = true
			}
		}
		require.True(t, check)
	})
	t.Run("Test state", func(t *testing.T) {
		test1 := "test1.java"
		outerFs.WriteFile(test1, []byte("test1"), 0644)

		info, err := outerFs.Stat(test1)
		require.NoError(t, err)
		require.Equal(t, "test1.java", info.Name())
	})
	t.Run("test open", func(t *testing.T) {
		test2 := "test2.java"
		outerFs.WriteFile(test2, []byte("test2"), 0644)

		info, err := outerFs.Open(test2)
		require.NoError(t, err)
		require.NotNil(t, info)
		fi, err := info.Stat()
		require.NoError(t, err)
		require.Equal(t, "test2.java", fi.Name())
	})

	t.Run("test openfile ", func(t *testing.T) {
		test3 := "test3.java"
		outerFs.WriteFile(test3, []byte("test3"), 0644)

		info, err := outerFs.OpenFile(test3, os.O_RDONLY, 0644)
		require.NoError(t, err)
		require.NotNil(t, info)
		fi, err := info.Stat()
		require.NoError(t, err)
		require.Equal(t, "test3.java", fi.Name())
	})

	t.Run("test exist virtual ext file", func(t *testing.T) {
		fileName := "test4.class"
		innerFs.WriteFile(fileName, []byte("test4"), 0644)
		file, err := outerFs.Open("test4.class")
		require.NoError(t, err)
		info, err := file.Stat()
		require.NoError(t, err)
		require.Equal(t, fileName, info.Name())
	})

	t.Run("test rename exist virtual ext file", func(t *testing.T) {
		fileName := `toRename.class`
		data := []byte(uuid.NewString())
		err := innerFs.WriteFile(fileName, data, 0644)
		require.NoError(t, err)
		err = outerFs.Rename(fileName, "toRename.java")
		require.Error(t, err)
	})

	t.Run("test rename to exist virtual ext file", func(t *testing.T) {
		to := `renameTo.class`
		origin := `origin.class`
		data := []byte(uuid.NewString())
		err := innerFs.WriteFile(to, data, 0644)
		require.NoError(t, err)
		innerFs.WriteFile(origin, data, 0644)
		require.NoError(t, err)
		err = outerFs.Rename(origin, to)
		require.Error(t, err)
	})

	t.Run("test open existed virtual ext file", func(t *testing.T) {
		name := uuid.NewString()[:4]
		fileName := name + ".java"

		data1 := []byte(uuid.NewString())
		err := innerFs.WriteFile(fileName, data1, 0644)
		require.NoError(t, err)
		data2 := []byte(uuid.NewString())
		err = innerFs.WriteFile(name+".class", data2, 0644)
		require.NoError(t, err)

		raw, err := outerFs.ReadFile(fileName)
		require.NoError(t, err)
		require.Equal(t, data1, raw)
	})
}

func TestUnifiedFs_Recursive(t *testing.T) {
	vf := NewVirtualFs()
	vf.AddFile("a/b/c.class", "c content")
	vf.AddFile("a/d.class", "d content")
	vf.AddFile("a/e.java", "e content")

	tmp := NewUnifiedFS(vf,
		WithUnifiedFsExtMap(".class", ".java"),
	)
	uf := NewUnifiedFS(tmp,
		WithUnifiedFsSeparator('$'),
	)

	var path []string
	var infoName []string
	var content []string
	Recursive(".", WithFileSystem(uf), WithFileStat(func(s string, info fs.FileInfo) error {
		t.Log("path is", s)
		path = append(path, s)
		t.Log("infoName is", info.Name())
		infoName = append(infoName, info.Name())
		data, err := uf.ReadFile(s)
		require.NoError(t, err)
		content = append(content, string(data))
		return nil
	}))
	expectedPath := []string{"a$b$c.java", "a$d.java", "a$e.java"}
	expectedName := []string{"c.java", "d.java", "e.java"}
	expectedContent := []string{"c content", "d content", "e content"}
	require.Equal(t, expectedPath, path)
	require.Equal(t, expectedName, infoName)
	require.Equal(t, expectedContent, content)
}
