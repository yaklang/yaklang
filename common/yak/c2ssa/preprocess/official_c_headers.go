package preprocess

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// OfficialCHeadersName is the thirdparty_bin / bin_cfg.yml registry name.
	OfficialCHeadersName = "c-std-headers"
	// OfficialCHeadersZipName is the on-disk pack basename under $YAKIT_HOME/c-headers.
	OfficialCHeadersZipName = "c-std-headers.zip"
)

var (
	autoDownloadOnce   sync.Once
	autoDownloadErr    error
	autoDownloadResult []string
)

// OfficialCHeadersAutoDownloadDisabled reports whether env disables auto fetch.
func OfficialCHeadersAutoDownloadDisabled() bool {
	v := strings.TrimSpace(os.Getenv("YAK_DISABLE_C_HEADERS_AUTO_DOWNLOAD"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

// OfficialCHeadersPackPath returns the expected install path for the official zip.
// Prefer thirdparty_bin.GetBinaryPath when the pack is already installed.
func OfficialCHeadersPackPath() (string, error) {
	if path, err := thirdparty_bin.GetBinaryPath(OfficialCHeadersName); err == nil && path != "" {
		return path, nil
	}
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

// EnsureOfficialCHeaders lazily installs the official pack via thirdparty_bin
// (bin_cfg.yml entry "c-std-headers") when missing, or Force-reinstalls.
func EnsureOfficialCHeaders(ctx context.Context, force bool) (packPath string, version string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := thirdparty_bin.EnsureInitialized(); err != nil {
		return "", "", utils.Wrap(err, "thirdparty_bin not ready for c-std-headers")
	}

	if !force {
		if p, getErr := thirdparty_bin.GetBinaryPath(OfficialCHeadersName); getErr == nil && p != "" {
			return p, FetchOfficialCHeadersVersion(ctx), nil
		}
	}

	if err := thirdparty_bin.Install(OfficialCHeadersName, &thirdparty_bin.InstallOptions{
		Context: ctx,
		Force:   force,
	}); err != nil {
		return "", "", utils.Wrap(err, "install official c-headers via thirdparty_bin")
	}

	p, err := thirdparty_bin.GetBinaryPath(OfficialCHeadersName)
	if err != nil {
		return "", "", utils.Wrap(err, "resolve installed official c-headers path")
	}
	return p, FetchOfficialCHeadersVersion(ctx), nil
}

// EnsureExternalIncludeDirs returns c-headers roots; if none exist, lazily downloads
// once through thirdparty_bin. On failure it degrades to project-local includes only.
func EnsureExternalIncludeDirs() []string {
	dir := consts.GetDefaultCHeadersDir()
	roots := collectCHeaderRoots(dir)
	if len(roots) > 0 || OfficialCHeadersAutoDownloadDisabled() {
		return roots
	}
	// Unit tests must not hit real OSS / thirdparty_bin install.
	if utils.InTestcase() {
		return roots
	}

	autoDownloadOnce.Do(func() {
		log.Infof("c-headers library missing under %s; lazy-install via thirdparty_bin (%s)", dir, OfficialCHeadersName)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		pack, ver, err := EnsureOfficialCHeaders(ctx, false)
		if err != nil {
			autoDownloadErr = err
			log.Errorf(
				"official c-headers lazy download failed, degrade to project-local includes only (no $YAKIT_HOME/c-headers): dir=%s name=%s err=%v",
				dir, OfficialCHeadersName, err,
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
	return collectCHeaderRoots(dir)
}

// LastOfficialCHeadersAutoDownloadError returns the last auto-download failure (if any).
func LastOfficialCHeadersAutoDownloadError() error {
	return autoDownloadErr
}

// FetchOfficialCHeadersVersion reads remote version.txt next to the zip URL (best-effort).
func FetchOfficialCHeadersVersion(ctx context.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	info, err := thirdparty_bin.GetDownloadInfo(OfficialCHeadersName)
	if err != nil || info == nil || strings.TrimSpace(info.URL) == "" {
		return ""
	}
	versionURL := siblingVersionURL(info.URL)
	if versionURL == "" {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
	if err != nil {
		return ""
	}
	client := utils.NewDefaultHTTPClient()
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

func siblingVersionURL(zipURL string) string {
	u, err := url.Parse(strings.TrimSpace(zipURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	u.Path = path.Join(path.Dir(u.Path), "version.txt")
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
