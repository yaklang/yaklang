package preprocess

import (
	"archive/zip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

func TestMain(m *testing.M) {
	// Existing preprocess unit tests must not hit real OSS via DefaultConfig().
	_ = os.Setenv("YAK_DISABLE_C_HEADERS_AUTO_DOWNLOAD", "1")
	os.Exit(m.Run())
}

func TestEnsureOfficialCHeaders_DownloadsWhenMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("YAKIT_HOME", home)
	t.Setenv("YAK_DISABLE_C_HEADERS_AUTO_DOWNLOAD", "0")

	zipDir := t.TempDir()
	srcZip := filepath.Join(zipDir, "pack.zip")
	f, err := os.Create(srcZip)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	w, err := zw.Create("stdio.h")
	require.NoError(t, err)
	_, err = w.Write([]byte("#define STDIO 1\n"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())
	raw, err := os.ReadFile(srcZip)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/c-headers/latest/c-std-headers.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(raw)
	})
	mux.HandleFunc("/c-headers/latest/version.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("auto-1\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	restore := SetOfficialCHeadersURLs(srv.URL+"/c-headers/latest/c-std-headers.zip", srv.URL+"/c-headers/latest/version.txt")
	t.Cleanup(restore)

	require.False(t, HasOfficialCHeadersLibrary())
	pack, ver, err := EnsureOfficialCHeaders(context.Background(), false)
	require.NoError(t, err)
	require.Equal(t, "auto-1", ver)
	require.FileExists(t, pack)
	require.True(t, HasOfficialCHeadersLibrary())

	roots := DetectExternalIncludeDirs()
	require.Contains(t, roots, consts.GetDefaultCHeadersDir())
	require.Contains(t, roots, pack)
}

func TestEnsureExternalIncludeDirs_AutoDownloadOnce(t *testing.T) {
	// Reset Once for this isolated process test via direct Ensure path already covered;
	// here we only assert Detect stays empty when auto-download is disabled.
	home := t.TempDir()
	t.Setenv("YAKIT_HOME", home)
	t.Setenv("YAK_DISABLE_C_HEADERS_AUTO_DOWNLOAD", "1")
	require.Empty(t, EnsureExternalIncludeDirs())
}

func TestEnsureOfficialCHeaders_NetworkFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("YAKIT_HOME", home)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	restore := SetOfficialCHeadersURLs(srv.URL+"/missing.zip", srv.URL+"/version.txt")
	t.Cleanup(restore)

	_, _, err := EnsureOfficialCHeaders(context.Background(), false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "download official c-headers from oss")
	require.False(t, HasOfficialCHeadersLibrary())
}
