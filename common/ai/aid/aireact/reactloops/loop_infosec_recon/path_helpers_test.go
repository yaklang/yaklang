package loop_infosec_recon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInfosecPickFirstHTTPURL(t *testing.T) {
	got, coerced, note := infosecPickFirstHTTPURL("http://10.0.0.1,https://10.0.0.1")
	require.True(t, coerced)
	require.Contains(t, note, "https")
	require.Equal(t, "https://10.0.0.1", got)

	got2, coerced2, _ := infosecPickFirstHTTPURL("http://10.0.0.1,http://10.0.0.2")
	require.True(t, coerced2)
	require.Equal(t, "http://10.0.0.1", got2)

	plain, coerced3, _ := infosecPickFirstHTTPURL("https://example.com/")
	require.False(t, coerced3)
	require.Equal(t, "https://example.com/", plain)
}

func TestInfosecResolveJsStaticPaths_CommaInDirName(t *testing.T) {
	dir := t.TempDir()
	commaDir := filepath.Join(dir, "10.0.0.1,https___host_crawl-js-collector_123")
	require.NoError(t, os.MkdirAll(commaDir, 0o755))

	paths, source, err := infosecResolveJsStaticPaths(commaDir, "", "", dir)
	require.NoError(t, err)
	require.Equal(t, "single local path (comma-safe)", source)
	require.Len(t, paths, 1)
	require.Equal(t, commaDir, paths[0])

	paths2, source2, err := infosecResolveJsStaticPaths("", commaDir, "", dir)
	require.NoError(t, err)
	require.Equal(t, "dir parameter", source2)
	require.Equal(t, commaDir, paths2[0])

	_, _, err = infosecResolveJsStaticPaths(commaDir, "", "", dir)
	require.NoError(t, err)
}

func TestInfosecResolveJsStaticPaths_AutoVerifiedDir(t *testing.T) {
	dir := t.TempDir()
	verified := filepath.Join(dir, "verified_js")
	require.NoError(t, os.MkdirAll(verified, 0o755))

	paths, source, err := infosecResolveJsStaticPaths("", "", verified, dir)
	require.NoError(t, err)
	require.Equal(t, "auto from crawl artifacts.verified_js_dir", source)
	require.Equal(t, verified, paths[0])
}

func TestInfosecResolveJsStaticPaths_CommaSplitMulti(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.js")
	b := filepath.Join(dir, "b.js")
	require.NoError(t, os.WriteFile(a, []byte("//"), 0o644))
	require.NoError(t, os.WriteFile(b, []byte("//"), 0o644))

	paths, source, err := infosecResolveJsStaticPaths(a+","+b, "", "", dir)
	require.NoError(t, err)
	require.Equal(t, "comma-separated paths", source)
	require.Len(t, paths, 2)
}
