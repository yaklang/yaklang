package thirdparty_bin

import (
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

// c-std-headers: official C header zip for c2ssa preprocess, kept as bin under
// install_root=c-headers (~/yakit-projects/c-headers/c-std-headers.zip).
// 关键词: c-std-headers, c-headers, bin_cfg, install_root, lazy download

const (
	cStdHeadersName            = "c-std-headers"
	cStdHeadersExpectedRoot    = "c-headers"
	cStdHeadersExpectedBinPath = "c-std-headers.zip"
	cStdHeadersExpectedURLTail = "/c-headers/latest/c-std-headers.zip"
	cStdHeadersExpectedIType   = "bin"
)

func findCStdHeadersDescriptor(t *testing.T) *BinaryDescriptor {
	t.Helper()
	cfg, err := LoadConfigFromEmbedded()
	require.NoError(t, err, "embedded bin_cfg.yml must parse")
	for _, b := range cfg.Binaries {
		if b.Name == cStdHeadersName {
			return b
		}
	}
	t.Fatalf("c-std-headers entry missing in bin_cfg.yml")
	return nil
}

func TestCStdHeaders_BinCfgEntry(t *testing.T) {
	desc := findCStdHeadersDescriptor(t)

	assert.Equal(t, cStdHeadersExpectedIType, desc.InstallType, "install_type")
	assert.Equal(t, cStdHeadersExpectedRoot, desc.InstallRoot,
		"install_root must be %q so the zip lands under ~/yakit-projects/c-headers/",
		cStdHeadersExpectedRoot)

	dl, ok := desc.DownloadInfoMap["*"]
	require.True(t, ok, `download_info_map must have "*" platform key`)
	assert.Equal(t, cStdHeadersExpectedBinPath, dl.BinPath,
		"bin_path must be the zip basename used by ListCHeaders / OpenExternalRoot")
	assert.Empty(t, dl.Pick, "bin install must not extract (keep zip as pack)")
	assert.Empty(t, dl.BinDir, "bin install must not use bin_dir")

	u, err := url.Parse(dl.URL)
	require.NoError(t, err, "URL must be parseable")
	assert.True(t, u.IsAbs(), "URL must be absolute after baseurl join, got %q", dl.URL)
	assert.Contains(t, dl.URL, cStdHeadersExpectedURLTail,
		"URL must end with %q (latest channel)", cStdHeadersExpectedURLTail)
}

func TestCStdHeaders_GetTargetPathUsesCHeadersRoot(t *testing.T) {
	desc := findCStdHeadersDescriptor(t)
	bi := &BaseInstaller{defaultInstallDir: t.TempDir(), downloadDir: t.TempDir()}

	target := bi.GetTargetPath(desc, nil)
	assert.Equal(t, filepath.Join(consts.GetDefaultCHeadersDir(), cStdHeadersExpectedBinPath), target)

	installDir := bi.GetInstallDir(desc, nil)
	assert.Empty(t, installDir, "bin without bin_dir has empty GetInstallDir")
}
