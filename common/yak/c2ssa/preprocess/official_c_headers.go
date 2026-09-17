package preprocess

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const OfficialCHeadersZipName = "c-std-headers.zip"

var (
	officialCHeadersZipURL     = "https://yaklang.oss-accelerate.aliyuncs.com/c-headers/latest/c-std-headers.zip"
	officialCHeadersVersionURL = "https://yaklang.oss-accelerate.aliyuncs.com/c-headers/latest/version.txt"

	autoDownloadOnce   sync.Once
	autoDownloadErr    error
	autoDownloadResult []string
)

// SetOfficialCHeadersURLs overrides OSS endpoints (tests). Returns a restore func.
func SetOfficialCHeadersURLs(zipURL, versionURL string) func() {
	oldZip, oldVer := officialCHeadersZipURL, officialCHeadersVersionURL
	if strings.TrimSpace(zipURL) != "" {
		officialCHeadersZipURL = zipURL
	}
	if strings.TrimSpace(versionURL) != "" {
		officialCHeadersVersionURL = versionURL
	}
	return func() {
		officialCHeadersZipURL, officialCHeadersVersionURL = oldZip, oldVer
	}
}

// OfficialCHeadersAutoDownloadDisabled reports whether env disables auto fetch.
func OfficialCHeadersAutoDownloadDisabled() bool {
	v := strings.TrimSpace(os.Getenv("YAK_DISABLE_C_HEADERS_AUTO_DOWNLOAD"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

// OfficialCHeadersPackPath returns $YAKIT_HOME/c-headers/c-std-headers.zip.
func OfficialCHeadersPackPath() (string, error) {
	base := consts.GetDefaultCHeadersDir()
	abs, err := filepath.Abs(base)
	if err != nil {
		return "", utils.Wrap(err, "resolve c-headers dir")
	}
	target, err := filepath.Abs(filepath.Join(abs, OfficialCHeadersZipName))
	if err != nil {
		return "", err
	}
	if target == abs || !utils.IsSubPath(target, abs) {
		return "", utils.Error("path escapes c-headers directory")
	}
	return target, nil
}

// HasOfficialCHeadersLibrary reports whether any usable header pack exists under c-headers.
func HasOfficialCHeadersLibrary() bool {
	return len(collectCHeaderRoots(consts.GetDefaultCHeadersDir())) > 0
}

// EnsureOfficialCHeaders downloads the official zip from OSS when missing (or Force).
// Returns the pack path on success / already present.
func EnsureOfficialCHeaders(ctx context.Context, force bool) (packPath string, version string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	dest, err := OfficialCHeadersPackPath()
	if err != nil {
		return "", "", err
	}
	if !force {
		if _, statErr := os.Stat(dest); statErr == nil {
			return dest, FetchOfficialCHeadersVersion(ctx), nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o777); err != nil {
		return "", "", utils.Wrap(err, "mkdir c-headers")
	}

	tmp := dest + ".download"
	_ = os.Remove(tmp)
	if err := downloadOfficialCHeadersZip(ctx, tmp); err != nil {
		_ = os.Remove(tmp)
		return "", "", err
	}
	if err := os.RemoveAll(dest); err != nil {
		_ = os.Remove(tmp)
		return "", "", utils.Wrap(err, "replace official c-headers pack")
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return "", "", utils.Wrap(err, "install official c-headers pack")
	}
	return dest, FetchOfficialCHeadersVersion(ctx), nil
}

func downloadOfficialCHeadersZip(ctx context.Context, localFile string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, officialCHeadersZipURL, nil)
	if err != nil {
		return utils.Wrapf(err, "build oss download request (%s)", officialCHeadersZipURL)
	}
	client := newOfficialCHeadersHTTPClient()
	rsp, err := client.Do(req)
	if err != nil {
		return utils.Wrapf(err, "download official c-headers from oss (%s)", officialCHeadersZipURL)
	}
	if rsp == nil || rsp.Body == nil {
		return utils.Errorf("download official c-headers from oss (%s): empty response", officialCHeadersZipURL)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode < 200 || rsp.StatusCode >= 300 {
		snip, _ := io.ReadAll(io.LimitReader(rsp.Body, 256))
		return utils.Errorf(
			"download official c-headers from oss (%s): unexpected status %d: %s",
			officialCHeadersZipURL, rsp.StatusCode, strings.TrimSpace(string(snip)),
		)
	}

	fp, err := os.OpenFile(localFile, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o666)
	if err != nil {
		return utils.Wrap(err, "create c-headers download temp file")
	}
	defer fp.Close()

	n, err := io.Copy(fp, rsp.Body)
	if err != nil {
		return utils.Wrapf(err, "download official c-headers from oss (%s)", officialCHeadersZipURL)
	}
	if n < 4 {
		return utils.Errorf("download official c-headers from oss (%s): response too small (%d bytes)", officialCHeadersZipURL, n)
	}
	if err := fp.Sync(); err != nil {
		return utils.Wrap(err, "sync c-headers download temp file")
	}
	if _, err := fp.Seek(0, io.SeekStart); err != nil {
		return utils.Wrap(err, "rewind c-headers download temp file")
	}
	var magic [2]byte
	if _, err := io.ReadFull(fp, magic[:]); err != nil {
		return utils.Wrapf(err, "read downloaded c-headers magic (%s)", officialCHeadersZipURL)
	}
	if magic[0] != 'P' || magic[1] != 'K' {
		return utils.Errorf("download official c-headers from oss (%s): content is not a zip archive", officialCHeadersZipURL)
	}
	return nil
}

// EnsureExternalIncludeDirs returns c-headers roots; if none exist, tries OSS download once.
// On network/OSS failure it degrades to an empty include list (project-local headers only)
// and emits an ERROR log so operators can see the fallback.
func EnsureExternalIncludeDirs() []string {
	dir := consts.GetDefaultCHeadersDir()
	roots := collectCHeaderRoots(dir)
	if len(roots) > 0 || OfficialCHeadersAutoDownloadDisabled() {
		return roots
	}

	autoDownloadOnce.Do(func() {
		log.Infof("c-headers library missing under %s; trying OSS download (%s)", dir, OfficialCHeadersZipName)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		pack, ver, err := EnsureOfficialCHeaders(ctx, false)
		if err != nil {
			autoDownloadErr = err
			log.Errorf(
				"official c-headers OSS download failed, degrade to project-local includes only (no $YAKIT_HOME/c-headers): dir=%s url=%s err=%v",
				dir, officialCHeadersZipURL, err,
			)
			autoDownloadResult = nil
			return
		}
		log.Infof("official c-headers ready: pack=%s version=%s", pack, ver)
		autoDownloadResult = collectCHeaderRoots(dir)
		if len(autoDownloadResult) == 0 {
			autoDownloadErr = utils.Errorf("downloaded pack %s but c-headers roots still empty", pack)
			log.Errorf(
				"official c-headers installed but unusable, degrade to project-local includes only: pack=%s err=%v",
				pack, autoDownloadErr,
			)
		}
	})
	if len(autoDownloadResult) > 0 {
		return append([]string(nil), autoDownloadResult...)
	}
	// Degraded path: keep whatever is on disk (usually empty) and continue preprocess.
	return collectCHeaderRoots(dir)
}

// LastOfficialCHeadersAutoDownloadError returns the last auto-download failure (if any).
func LastOfficialCHeadersAutoDownloadError() error {
	return autoDownloadErr
}

func newOfficialCHeadersHTTPClient() *http.Client {
	client := utils.NewDefaultHTTPClient()
	client.Timeout = 10 * time.Minute
	return client
}

// FetchOfficialCHeadersVersion reads the remote version.txt (best-effort).
func FetchOfficialCHeadersVersion(ctx context.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, officialCHeadersVersionURL, nil)
	if err != nil {
		return ""
	}
	client := newOfficialCHeadersHTTPClient()
	client.Timeout = 15 * time.Second
	rsp, err := client.Do(req)
	if err != nil || rsp == nil || rsp.Body == nil {
		return ""
	}
	defer rsp.Body.Close()
	if rsp.StatusCode < 200 || rsp.StatusCode >= 300 {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(rsp.Body, 256))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}
