package preprocess

import (
	"archive/zip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestMain(m *testing.M) {
	// Existing preprocess unit tests must not hit real OSS via DefaultConfig().
	_ = os.Setenv("YAK_DISABLE_C_HEADERS_AUTO_DOWNLOAD", "1")
	os.Exit(m.Run())
}

func ensureThirdpartyReady(t *testing.T) {
	t.Helper()
	require.NoError(t, yakit.CallPostInitDatabase())
	require.NoError(t, thirdparty_bin.EnsureInitialized())
}

// overrideOfficialCHeadersURL re-registers c-std-headers against a local httptest URL
// so Install stays offline. Restores the embedded descriptor on cleanup.
func overrideOfficialCHeadersURL(t *testing.T, zipURL string) {
	t.Helper()
	ensureThirdpartyReady(t)

	orig, err := thirdparty_bin.GetDescriptor(OfficialCHeadersName)
	require.NoError(t, err)
	origCopy := *orig
	if orig.DownloadInfoMap != nil {
		origCopy.DownloadInfoMap = make(map[string]*thirdparty_bin.DownloadInfo, len(orig.DownloadInfoMap))
		for k, v := range orig.DownloadInfoMap {
			if v == nil {
				continue
			}
			cp := *v
			origCopy.DownloadInfoMap[k] = &cp
		}
	}

	require.NoError(t, thirdparty_bin.Register(&thirdparty_bin.BinaryDescriptor{
		Name:        OfficialCHeadersName,
		Description: orig.Description,
		Tags:        append([]string(nil), orig.Tags...),
		Version:     orig.Version,
		InstallType: "bin",
		InstallRoot: "c-headers",
		DownloadInfoMap: map[string]*thirdparty_bin.DownloadInfo{
			"*": {
				URL:     zipURL,
				BinPath: OfficialCHeadersZipName,
			},
		},
	}))
	t.Cleanup(func() {
		_ = thirdparty_bin.Register(&origCopy)
	})
}

func TestEnsureOfficialCHeaders_DownloadsWhenMissing(t *testing.T) {
	// Init DB under ambient YAKIT_HOME first so t.TempDir cleanup is not blocked by sqlite.
	ensureThirdpartyReady(t)

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
		w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write(raw)
	})
	mux.HandleFunc("/c-headers/latest/version.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("auto-1\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	overrideOfficialCHeadersURL(t, srv.URL+"/c-headers/latest/c-std-headers.zip")

	require.False(t, HasOfficialCHeadersLibrary())
	pack, ver, err := EnsureOfficialCHeaders(context.Background(), false)
	require.NoError(t, err)
	require.Equal(t, "auto-1", ver)
	require.FileExists(t, pack)
	require.Equal(t, OfficialCHeadersZipName, filepath.Base(pack))
	require.True(t, HasOfficialCHeadersLibrary())

	roots := DetectExternalIncludeDirs()
	require.Contains(t, roots, consts.GetDefaultCHeadersDir())
	require.Contains(t, roots, pack)
}

func TestEnsureExternalIncludeDirs_AutoDownloadOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("YAKIT_HOME", home)
	t.Setenv("YAK_DISABLE_C_HEADERS_AUTO_DOWNLOAD", "1")
	require.Empty(t, EnsureExternalIncludeDirs())
}

func TestEnsureOfficialCHeaders_NetworkFailure(t *testing.T) {
	ensureThirdpartyReady(t)

	home := t.TempDir()
	t.Setenv("YAKIT_HOME", home)
	// Closed port: thirdparty_bin DownloadFile may accept HTTP error bodies as "success".
	overrideOfficialCHeadersURL(t, "http://127.0.0.1:1/c-std-headers.zip")

	_, _, err := EnsureOfficialCHeaders(context.Background(), false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "install official c-headers via thirdparty_bin")
	require.False(t, HasOfficialCHeadersLibrary())
}
