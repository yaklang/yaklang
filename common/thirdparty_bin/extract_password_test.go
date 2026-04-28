package thirdparty_bin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zip "github.com/yaklang/yaklang/common/utils/zipx"
)

// 创建带密码的 zip
// 关键词: 测试 install 加密 zip
func makeEncryptedZip(t *testing.T, files map[string]string, password string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		fw, err := w.Encrypt(name, password, zip.AES256Encryption)
		require.NoError(t, err)
		_, err = fw.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// TestExtractFileWithPassword_ZipPick 验证带密码 zip 的多种 pick 模式
// 关键词: ExtractFileWithPassword, install pick
func TestExtractFileWithPassword_ZipPick(t *testing.T) {
	files := map[string]string{
		"build/main.exe":      "binary",
		"build/lib/dll.dll":   "library",
		"build/config.ini":    "config",
		"build/sub/extra.txt": "extra",
	}
	password := "install-pwd"
	zipData := makeEncryptedZip(t, files, password)

	tempDir := t.TempDir()
	zipPath := filepath.Join(tempDir, "pkg.zip")
	require.NoError(t, os.WriteFile(zipPath, zipData, 0o644))

	t.Run("Pick all", func(t *testing.T) {
		dest := filepath.Join(tempDir, "all")
		require.NoError(t, ExtractFileWithPassword(zipPath, dest, ".zip", "*", true, password))

		got, err := os.ReadFile(filepath.Join(dest, "build", "main.exe"))
		require.NoError(t, err)
		assert.Equal(t, "binary", string(got))
	})

	t.Run("Pick build/", func(t *testing.T) {
		dest := filepath.Join(tempDir, "dir")
		require.NoError(t, ExtractFileWithPassword(zipPath, dest, ".zip", "build/", true, password))

		got, err := os.ReadFile(filepath.Join(dest, "build", "config.ini"))
		require.NoError(t, err)
		assert.Equal(t, "config", string(got))
	})

	t.Run("Pick build/* (contents)", func(t *testing.T) {
		dest := filepath.Join(tempDir, "contents")
		require.NoError(t, ExtractFileWithPassword(zipPath, dest, ".zip", "build/*", true, password))

		got, err := os.ReadFile(filepath.Join(dest, "main.exe"))
		require.NoError(t, err)
		assert.Equal(t, "binary", string(got))
	})

	t.Run("Pick single file", func(t *testing.T) {
		target := filepath.Join(tempDir, "single", "config.ini")
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
		require.NoError(t, ExtractFileWithPassword(zipPath, target, ".zip", "config.ini", false, password))

		got, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, "config", string(got))
	})

	t.Run("Missing password fails", func(t *testing.T) {
		dest := filepath.Join(tempDir, "fail")
		err := ExtractFileWithPassword(zipPath, dest, ".zip", "*", true, "")
		require.Error(t, err)
		assert.True(t,
			strings.Contains(err.Error(), "encrypted") || strings.Contains(err.Error(), "password"),
			"expected encryption error, got: %v", err,
		)
	})
}

// TestInstallFromHTTP 端到端：从 HTTP 下载带密码 zip 到本地，再通过 install 入口解开
// 关键词: 互联网下载安装, install 加密 zip
func TestInstallFromHTTP(t *testing.T) {
	files := map[string]string{
		"build/binary":   "ELF...",
		"build/help.txt": "usage docs",
	}
	password := "remote-secret"
	zipData := makeEncryptedZip(t, files, password)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipData)
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/pkg.zip")
	require.NoError(t, err)
	body := new(bytes.Buffer)
	_, err = body.ReadFrom(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)

	tempDir := t.TempDir()
	downloaded := filepath.Join(tempDir, "downloaded.zip")
	require.NoError(t, os.WriteFile(downloaded, body.Bytes(), 0o644))

	dest := filepath.Join(tempDir, "install")
	require.NoError(t, ExtractFileWithPassword(downloaded, dest, ".zip", "build/*", true, password))

	got, err := os.ReadFile(filepath.Join(dest, "help.txt"))
	require.NoError(t, err)
	assert.Equal(t, "usage docs", string(got))
}
