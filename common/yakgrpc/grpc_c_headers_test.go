package yakgrpc

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
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func withTempCHeadersHome(t *testing.T) {
	t.Helper()
	t.Setenv("YAKIT_HOME", t.TempDir())
	_ = consts.GetDefaultCHeadersDir()
}

func writeZip(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	defer f.Close()
	w := zip.NewWriter(f)
	for name, body := range files {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
}

func TestCHeaders_ImportListPreviewDelete(t *testing.T) {
	withTempCHeadersHome(t)
	s := &Server{}
	ctx := context.Background()

	dirResp, err := s.GetCHeadersDir(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.DirExists(t, dirResp.GetDir())

	srcDir := t.TempDir()
	headerPath := filepath.Join(srcDir, "demo.h")
	require.NoError(t, os.WriteFile(headerPath, []byte("#define DEMO 1\n"), 0o644))

	imp, err := s.ImportCHeaderPack(ctx, &ypb.ImportCHeaderPackRequest{
		LocalPath: headerPath,
		DestName:  "demo.h",
	})
	require.NoError(t, err)
	require.True(t, imp.GetOk(), imp.GetReason())

	list, err := s.ListCHeaders(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.NotEmpty(t, list.GetPacks())
	found := false
	for _, p := range list.GetPacks() {
		if p.GetName() == "demo.h" {
			found = true
			require.Equal(t, "file", p.GetKind())
		}
	}
	require.True(t, found)

	preview, err := s.PreviewCHeaderFile(ctx, &ypb.PreviewCHeaderFileRequest{
		RelativePath: "demo.h",
	})
	require.NoError(t, err)
	require.Contains(t, string(preview.GetContent()), "DEMO")
	require.False(t, preview.GetTruncated())

	del, err := s.DeleteCHeaderPack(ctx, &ypb.DeleteCHeaderPackRequest{Name: "demo.h"})
	require.NoError(t, err)
	require.True(t, del.GetOk(), del.GetReason())
	require.NoFileExists(t, filepath.Join(dirResp.GetDir(), "demo.h"))
}

func TestCHeaders_ZipEntriesAndExtract(t *testing.T) {
	withTempCHeadersHome(t)
	s := &Server{}
	ctx := context.Background()

	zipPath := filepath.Join(t.TempDir(), "pack.zip")
	writeZip(t, zipPath, map[string]string{
		"include/sys/queue.h": "#define QUEUE 1\n",
		"include/stdio.h":     "#define STDIO 1\n",
	})

	imp, err := s.ImportCHeaderPack(ctx, &ypb.ImportCHeaderPackRequest{
		LocalPath: zipPath,
	})
	require.NoError(t, err)
	require.True(t, imp.GetOk(), imp.GetReason())

	entries, err := s.ListCHeaderEntries(ctx, &ypb.ListCHeaderEntriesRequest{PackName: "pack.zip"})
	require.NoError(t, err)
	require.NotEmpty(t, entries.GetEntries())
	var includeDir *ypb.CHeaderEntry
	for _, e := range entries.GetEntries() {
		if e.GetName() == "include" {
			includeDir = e
		}
	}
	require.NotNil(t, includeDir)
	require.True(t, includeDir.GetIsDir())

	child, err := s.ListCHeaderEntries(ctx, &ypb.ListCHeaderEntriesRequest{
		PackName:     "pack.zip",
		RelativePath: includeDir.GetRelativePath(),
	})
	require.NoError(t, err)
	var stdio *ypb.CHeaderEntry
	for _, e := range child.GetEntries() {
		if e.GetName() == "stdio.h" {
			stdio = e
		}
	}
	require.NotNil(t, stdio)

	preview, err := s.PreviewCHeaderFile(ctx, &ypb.PreviewCHeaderFileRequest{
		PackName:     "pack.zip",
		RelativePath: stdio.GetRelativePath(),
	})
	require.NoError(t, err)
	require.Contains(t, string(preview.GetContent()), "STDIO")

	imp2, err := s.ImportCHeaderPack(ctx, &ypb.ImportCHeaderPackRequest{
		LocalPath:  zipPath,
		DestName:   "pack.zip",
		ExtractZip: true,
	})
	require.NoError(t, err)
	require.True(t, imp2.GetOk(), imp2.GetReason())
	dirResp, err := s.GetCHeadersDir(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.DirExists(t, filepath.Join(dirResp.GetDir(), "pack"))
}

func TestCHeaders_PathTraversalRejected(t *testing.T) {
	withTempCHeadersHome(t)
	s := &Server{}
	ctx := context.Background()

	del, err := s.DeleteCHeaderPack(ctx, &ypb.DeleteCHeaderPackRequest{Name: "../secret"})
	require.NoError(t, err)
	require.False(t, del.GetOk())

	del, err = s.DeleteCHeaderPack(ctx, &ypb.DeleteCHeaderPackRequest{Name: ".."})
	require.NoError(t, err)
	require.False(t, del.GetOk())

	_, err = s.ListCHeaderEntries(ctx, &ypb.ListCHeaderEntriesRequest{
		RelativePath: "../etc/passwd",
	})
	require.Error(t, err)

	_, err = s.PreviewCHeaderFile(ctx, &ypb.PreviewCHeaderFileRequest{
		RelativePath: "../secret.h",
	})
	require.Error(t, err)
}

func TestCHeaders_ImportDirectory(t *testing.T) {
	withTempCHeadersHome(t)
	s := &Server{}
	ctx := context.Background()

	src := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sys"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "sys", "queue.h"), []byte("#define Q 1\n"), 0o644))

	imp, err := s.ImportCHeaderPack(ctx, &ypb.ImportCHeaderPackRequest{
		LocalPath: src,
		DestName:  "mypack",
	})
	require.NoError(t, err)
	require.True(t, imp.GetOk(), imp.GetReason())

	preview, err := s.PreviewCHeaderFile(ctx, &ypb.PreviewCHeaderFileRequest{
		PackName:     "mypack",
		RelativePath: "sys/queue.h",
	})
	require.NoError(t, err)
	require.Contains(t, string(preview.GetContent()), "#define Q")
}

func TestCHeaders_DownloadOfficial(t *testing.T) {
	withTempCHeadersHome(t)
	s := &Server{}
	ctx := context.Background()

	zipPath := filepath.Join(t.TempDir(), "src.zip")
	writeZip(t, zipPath, map[string]string{"stdio.h": "#define STDIO 1\n"})
	raw, err := os.ReadFile(zipPath)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/c-headers/latest/c-std-headers.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(raw)
	})
	mux.HandleFunc("/c-headers/latest/version.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("1.0.0-test\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	oldZip, oldVer := officialCHeadersZipURL, officialCHeadersVersionURL
	officialCHeadersZipURL = srv.URL + "/c-headers/latest/c-std-headers.zip"
	officialCHeadersVersionURL = srv.URL + "/c-headers/latest/version.txt"
	t.Cleanup(func() {
		officialCHeadersZipURL = oldZip
		officialCHeadersVersionURL = oldVer
	})

	first, err := s.DownloadOfficialCHeaders(ctx, &ypb.DownloadOfficialCHeadersRequest{})
	require.NoError(t, err)
	require.True(t, first.GetOk(), first.GetReason())
	require.Equal(t, "1.0.0-test", first.GetVersion())
	require.FileExists(t, first.GetPackPath())
	require.Equal(t, "c-std-headers.zip", filepath.Base(first.GetPackPath()))

	dup, err := s.DownloadOfficialCHeaders(ctx, &ypb.DownloadOfficialCHeadersRequest{})
	require.NoError(t, err)
	require.False(t, dup.GetOk())

	again, err := s.DownloadOfficialCHeaders(ctx, &ypb.DownloadOfficialCHeadersRequest{Force: true})
	require.NoError(t, err)
	require.True(t, again.GetOk(), again.GetReason())

	preview, err := s.PreviewCHeaderFile(ctx, &ypb.PreviewCHeaderFileRequest{
		PackName:     "c-std-headers.zip",
		RelativePath: "stdio.h",
	})
	require.NoError(t, err)
	require.Contains(t, string(preview.GetContent()), "STDIO")
}
