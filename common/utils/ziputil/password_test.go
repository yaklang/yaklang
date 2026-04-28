package ziputil

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
// 关键词: 测试加密 zip, 内存生成
func createEncryptedZip(t *testing.T, files map[string]string, password string, method EncryptionMethod) []byte {
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
// 关键词: 测试明文 zip, 兼容性
func createPlainZip(t *testing.T, files map[string]string) []byte {
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

func TestExtractWithPassword(t *testing.T) {
	files := map[string]string{
		"a.txt":         "Hello AES",
		"sub/b.txt":     "Inside Sub",
		"sub/dir/c.log": "Log Content",
	}
	password := "p@ssw0rd!"

	for _, method := range []struct {
		name string
		m    EncryptionMethod
	}{
		{"ZipCrypto", StandardEncryption},
		{"AES128", AES128Encryption},
		{"AES192", AES192Encryption},
		{"AES256", AES256Encryption},
	} {
		method := method
		t.Run(method.name, func(t *testing.T) {
			data := createEncryptedZip(t, files, password, method.m)

			t.Run("ExtractFileFromRawWithOptions", func(t *testing.T) {
				content, err := ExtractFileFromRawWithOptions(data, "a.txt", WithExtractPassword(password))
				require.NoError(t, err)
				assert.Equal(t, "Hello AES", string(content))
			})

			t.Run("ExtractFileFromRawWithOptions wrong password", func(t *testing.T) {
				_, err := ExtractFileFromRawWithOptions(data, "a.txt", WithExtractPassword("wrong"))
				assert.Error(t, err)
			})

			t.Run("ExtractFileFromRawWithOptions missing password", func(t *testing.T) {
				_, err := ExtractFileFromRawWithOptions(data, "a.txt")
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "encrypted")
			})

			t.Run("ExtractFilesFromRawWithOptions", func(t *testing.T) {
				results, err := ExtractFilesFromRawWithOptions(data,
					[]string{"a.txt", "sub/b.txt"},
					WithExtractPassword(password),
				)
				require.NoError(t, err)
				assert.Len(t, results, 2)
				for _, r := range results {
					assert.NoError(t, r.Error, "extracting %s", r.FileName)
					assert.Equal(t, files[r.FileName], string(r.Content))
				}
			})

			t.Run("ExtractByPatternFromRawWithOptions", func(t *testing.T) {
				results, err := ExtractByPatternFromRawWithOptions(data, "*.txt", WithExtractPassword(password))
				require.NoError(t, err)
				assert.GreaterOrEqual(t, len(results), 1)
				for _, r := range results {
					assert.NoError(t, r.Error, "pattern matched %s", r.FileName)
					if r.Error == nil {
						assert.Equal(t, files[r.FileName], string(r.Content))
					}
				}
			})
		})
	}
}

func TestDecompressWithPassword(t *testing.T) {
	files := map[string]string{
		"top.txt":        "Top File",
		"nested/inside":  "Nested Content",
		"nested/dir.txt": "Dir Text",
	}
	password := "secret"
	data := createEncryptedZip(t, files, password, AES256Encryption)

	tempDir := t.TempDir()
	zipPath := filepath.Join(tempDir, "in.zip")
	require.NoError(t, os.WriteFile(zipPath, data, 0o644))

	dest := filepath.Join(tempDir, "out")

	t.Run("with password ok", func(t *testing.T) {
		require.NoError(t, DeCompressWithOptions(zipPath, dest, WithDecompressPassword(password)))

		got, err := os.ReadFile(filepath.Join(dest, "top.txt"))
		require.NoError(t, err)
		assert.Equal(t, "Top File", string(got))

		got, err = os.ReadFile(filepath.Join(dest, "nested", "dir.txt"))
		require.NoError(t, err)
		assert.Equal(t, "Dir Text", string(got))
	})

	t.Run("missing password", func(t *testing.T) {
		miss := filepath.Join(tempDir, "out_missing")
		err := DeCompressFromRawWithOptions(data, miss)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "encrypted")
	})

	t.Run("wrong password", func(t *testing.T) {
		wrong := filepath.Join(tempDir, "out_wrong")
		err := DeCompressFromRawWithOptions(data, wrong, WithDecompressPassword("nope"))
		assert.Error(t, err)
	})
}

func TestCompressByNameWithOptions_RoundTrip(t *testing.T) {
	tempDir := t.TempDir()

	src := filepath.Join(tempDir, "secret.txt")
	require.NoError(t, os.WriteFile(src, []byte("hello world"), 0o644))

	zipPath := filepath.Join(tempDir, "out.zip")
	require.NoError(t, CompressByNameWithOptions(
		[]string{src}, zipPath,
		WithCompressPassword("topsecret"),
		WithCompressEncryption(AES256Encryption),
	))

	dest := filepath.Join(tempDir, "extracted")
	require.NoError(t, DeCompressWithOptions(zipPath, dest, WithDecompressPassword("topsecret")))

	// 找到解压出的文件并校验内容
	var found string
	require.NoError(t, filepath.Walk(dest, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, "secret.txt") {
			found = path
		}
		return nil
	}))
	require.NotEmpty(t, found, "extracted file not found")
	got, err := os.ReadFile(found)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(got))
}

func TestGrepWithPassword(t *testing.T) {
	files := map[string]string{
		"alpha.txt": "first line\nsecret marker here\nlast",
		"beta.log":  "log:start\nnope",
	}
	password := "grep-secret"
	data := createEncryptedZip(t, files, password, AES256Encryption)

	t.Run("missing password fails to read", func(t *testing.T) {
		results, err := GrepRawSubString(data, "marker")
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("with password matches content", func(t *testing.T) {
		results, err := GrepRawSubString(data, "marker", WithGrepPassword(password))
		require.NoError(t, err)
		require.NotEmpty(t, results)
		assert.Equal(t, "alpha.txt", results[0].FileName)
		assert.Contains(t, results[0].Line, "marker")
	})

	t.Run("regex with password", func(t *testing.T) {
		results, err := GrepRawRegexp(data, `secret\s+marker`, WithGrepPassword(password))
		require.NoError(t, err)
		require.NotEmpty(t, results)
	})
}

func TestZipGrepSearcher_Password(t *testing.T) {
	files := map[string]string{
		"a/one.txt": "needle in haystack",
		"a/two.txt": "no match here",
	}
	password := "searcher-pwd"
	data := createEncryptedZip(t, files, password, AES256Encryption)

	searcher, err := NewZipGrepSearcherFromRaw(data)
	require.NoError(t, err)
	searcher.SetPassword(password)

	results, err := searcher.GrepSubString("needle")
	require.NoError(t, err)
	require.NotEmpty(t, results)
	assert.Equal(t, "a/one.txt", results[0].FileName)

	t.Run("preload", func(t *testing.T) {
		preload, err := NewZipGrepSearcherFromRaw(data)
		require.NoError(t, err)
		preload.SetPassword(password)
		preload.WithCacheAll(true)
		assert.GreaterOrEqual(t, len(preload.GetCachedFiles()), 2)
	})
}

func TestPlainZipBackwardCompatibility(t *testing.T) {
	files := map[string]string{
		"plain.txt": "no encryption",
		"another":   "still plain",
	}
	data := createPlainZip(t, files)

	t.Run("ExtractFileFromRawWithOptions without password works", func(t *testing.T) {
		content, err := ExtractFileFromRawWithOptions(data, "plain.txt")
		require.NoError(t, err)
		assert.Equal(t, "no encryption", string(content))
	})

	t.Run("ExtractFileFromRawWithOptions with stale password works", func(t *testing.T) {
		// 未加密 zip 即使带密码也不应失败
		content, err := ExtractFileFromRawWithOptions(data, "plain.txt", WithExtractPassword("ignored"))
		require.NoError(t, err)
		assert.Equal(t, "no encryption", string(content))
	})

	t.Run("Grep without password works on plain zip", func(t *testing.T) {
		results, err := GrepRawSubString(data, "encryption")
		require.NoError(t, err)
		require.NotEmpty(t, results)
	})

	t.Run("Existing legacy ExtractFile still works", func(t *testing.T) {
		content, err := ExtractFileFromRaw(data, "plain.txt")
		require.NoError(t, err)
		assert.Equal(t, "no encryption", string(content))
	})
}

// TestCompressRawMapWithOptions 验证内存压缩支持密码
// 关键词: 测试 CompressRawMapWithOptions
func TestCompressRawMapWithOptions(t *testing.T) {
	files := map[string]interface{}{
		"x.txt": "x content",
		"y.txt": []byte("y content"),
	}
	zipBytes, err := CompressRawMapWithOptions(files,
		WithCompressPassword("memo"),
		WithCompressEncryption(AES256Encryption),
	)
	require.NoError(t, err)
	require.NotEmpty(t, zipBytes)

	got, err := ExtractFileFromRawWithOptions(zipBytes, "x.txt", WithExtractPassword("memo"))
	require.NoError(t, err)
	assert.Equal(t, "x content", string(got))

	_, err = ExtractFileFromRawWithOptions(zipBytes, "y.txt")
	assert.Error(t, err, "expect error when no password supplied for encrypted entry")
}

// TestHTTPDownloadAndDecrypt 集成场景：模拟从 HTTP 下载带密码 zip 后解压安装
// 关键词: HTTP 下载 zip, install 流程, 密码解压
func TestHTTPDownloadAndDecrypt(t *testing.T) {
	files := map[string]string{
		"bin/tool":      "#!/bin/sh\necho ok\n",
		"config/v1.ini": "[default]\nkey=value",
		"README.md":     "encrypted bundle",
	}
	password := "install-secret"
	zipBytes := createEncryptedZip(t, files, password, AES256Encryption)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipBytes)
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/bundle.zip")
	require.NoError(t, err)
	defer resp.Body.Close()
	body := new(bytes.Buffer)
	_, err = body.ReadFrom(resp.Body)
	require.NoError(t, err)

	dest := filepath.Join(t.TempDir(), "install")
	require.NoError(t, DeCompressFromRawWithOptions(body.Bytes(), dest, WithDecompressPassword(password)))

	got, err := os.ReadFile(filepath.Join(dest, "config", "v1.ini"))
	require.NoError(t, err)
	assert.Equal(t, "[default]\nkey=value", string(got))

	got, err = os.ReadFile(filepath.Join(dest, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "encrypted bundle", string(got))
}
