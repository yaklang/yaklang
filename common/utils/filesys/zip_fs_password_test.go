package filesys

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zip "github.com/yaklang/yaklang/common/utils/zipx"
)

// 创建带密码的 zip
// 关键词: 测试 ZipFS 加密
func mustEncryptedZip(t *testing.T, files map[string]string, password string, method zip.EncryptionMethod) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		fw, err := w.Encrypt(name, password, method)
		require.NoError(t, err)
		_, err = fw.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// 创建未加密 zip
// 关键词: 测试 ZipFS 兼容
func mustPlainZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestZipFS_PasswordReadFile(t *testing.T) {
	files := map[string]string{
		"a.txt":     "alpha",
		"b/c.txt":   "deep",
		"top.json":  "{\"k\":1}",
	}
	password := "fs-pwd"
	data := mustEncryptedZip(t, files, password, zip.AES256Encryption)

	t.Run("with password ok", func(t *testing.T) {
		zfs, err := NewZipFSFromStringWithOptions(string(data), WithZipFSPassword(password))
		require.NoError(t, err)

		got, err := zfs.ReadFile("a.txt")
		require.NoError(t, err)
		assert.Equal(t, "alpha", string(got))

		got, err = zfs.ReadFile("b/c.txt")
		require.NoError(t, err)
		assert.Equal(t, "deep", string(got))
	})

	t.Run("missing password", func(t *testing.T) {
		zfs, err := NewZipFSFromStringWithOptions(string(data))
		require.NoError(t, err)
		_, err = zfs.ReadFile("a.txt")
		assert.Error(t, err)
	})

	t.Run("wrong password", func(t *testing.T) {
		zfs, err := NewZipFSFromStringWithOptions(string(data), WithZipFSPassword("xxx"))
		require.NoError(t, err)
		_, err = zfs.ReadFile("a.txt")
		assert.Error(t, err)
	})

	t.Run("legacy NewZipFSFromString reads plain zip", func(t *testing.T) {
		plain := mustPlainZip(t, files)
		zfs, err := NewZipFSFromString(string(plain))
		require.NoError(t, err)
		got, err := zfs.ReadFile("a.txt")
		require.NoError(t, err)
		assert.Equal(t, "alpha", string(got))
	})
}

func TestZipFS_PasswordReadDirAndStat(t *testing.T) {
	files := map[string]string{
		"docs/readme.md": "readme",
		"docs/list.txt":  "items",
		"main.go":        "package main",
	}
	password := "list"
	data := mustEncryptedZip(t, files, password, zip.AES256Encryption)

	zfs, err := NewZipFSFromStringWithOptions(string(data), WithZipFSPassword(password))
	require.NoError(t, err)

	t.Run("ReadDir", func(t *testing.T) {
		entries, err := zfs.ReadDir("docs")
		require.NoError(t, err)
		got := map[string]bool{}
		for _, e := range entries {
			got[e.Name()] = true
		}
		assert.True(t, got["readme.md"])
		assert.True(t, got["list.txt"])
	})

	t.Run("Stat file", func(t *testing.T) {
		info, err := zfs.Stat("main.go")
		require.NoError(t, err)
		assert.False(t, info.IsDir())
	})

	t.Run("Stat directory", func(t *testing.T) {
		info, err := zfs.Stat("docs")
		require.NoError(t, err)
		assert.True(t, info.IsDir() || info.Mode()&fs.ModeDir != 0)
	})

	t.Run("Open and read", func(t *testing.T) {
		f, err := zfs.Open("main.go")
		require.NoError(t, err)
		buf := make([]byte, 32)
		n, _ := f.Read(buf)
		f.Close()
		assert.Equal(t, "package main", string(buf[:n]))
	})
}

func TestZipFS_PasswordFromLocal(t *testing.T) {
	files := map[string]string{
		"hello.txt": "world",
	}
	password := "local-pwd"
	data := mustEncryptedZip(t, files, password, zip.AES256Encryption)

	tempDir := t.TempDir()
	zipPath := filepath.Join(tempDir, "in.zip")
	require.NoError(t, os.WriteFile(zipPath, data, 0o644))

	zfs, err := NewZipFSFromLocalWithOptions(zipPath, WithZipFSPassword(password))
	require.NoError(t, err)
	defer zfs.Close()

	got, err := zfs.ReadFile("hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "world", string(got))
}

func TestZipFS_PasswordRunTimeSet(t *testing.T) {
	files := map[string]string{
		"runtime.txt": "later",
	}
	password := "later-pwd"
	data := mustEncryptedZip(t, files, password, zip.AES256Encryption)

	zfs, err := NewZipFSFromString(string(data))
	require.NoError(t, err)
	zfs.SetPassword(password)

	got, err := zfs.ReadFile("runtime.txt")
	require.NoError(t, err)
	assert.Equal(t, "later", string(got))
}
