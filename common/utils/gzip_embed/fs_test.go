package gzip_embed

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"embed"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

// Reuse the existing embedded test archive as the source; the supported decode
// hook supplies a generated archive so tests can exercise directories and errors.
//
//go:embed test/static.tar.gz
var testArchive embed.FS

func archiveFixture(t *testing.T, cached bool) *PreprocessingEmbed {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, entry := range []struct {
		name, data string
		dir        bool
	}{
		{"dir/", "", true}, {"empty/", "", true}, {"dir/app.js", "console.log('hello');", false}, {"dir/plain.txt", "plain", false}, {"dir/z.txt", "last", false},
	} {
		kind := byte(tar.TypeReg)
		if entry.dir {
			kind = tar.TypeDir
		}
		if err := tw.WriteHeader(&tar.Header{Name: entry.name, Size: int64(len(entry.data)), Mode: 0644, Typeflag: kind}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(entry.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := NewPreprocessingEmbedWithDecode(&testArchive, "test/static.tar.gz", cached, func([]byte) ([]byte, error) { return buf.Bytes(), nil })
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestArchiveFilesystemContract(t *testing.T) {
	for _, cache := range []bool{false, true} {
		f := archiveFixture(t, cache)
		if err := fstest.TestFS(f, "dir/app.js", "dir/plain.txt", "dir/z.txt", "empty"); err != nil {
			t.Fatal(err)
		}
		sub, err := fs.Sub(f, "dir")
		if err != nil {
			t.Fatal(err)
		}
		data, err := fs.ReadFile(sub, "app.js")
		if err != nil || string(data) != "console.log('hello');" {
			t.Fatalf("%q %v", data, err)
		}
		for _, name := range []string{"../dir/app.js", "/dir/app.js", "dir/../dir/app.js", ""} {
			if _, err := f.Open(name); !errors.Is(err, fs.ErrInvalid) {
				t.Fatalf("%q: %v", name, err)
			}
		}
		if _, err := f.ReadFile("missing.gz"); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		if _, err := f.ReadDir("missing"); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				data, err := f.ReadFile("dir/app.js")
				if err != nil || string(data) != "console.log('hello');" {
					t.Errorf("%q %v", data, err)
				}
				if len(data) > 0 {
					data[0] = 'X'
				}
			}()
		}
		wg.Wait()
	}
}

func TestArchiveHTTPAndSeek(t *testing.T) {
	f := archiveFixture(t, false)
	handler := http.FileServer(http.FS(f))
	for _, tc := range []struct {
		method, rng string
		code        int
		body        string
	}{
		{"GET", "", 200, "console.log('hello');"}, {"HEAD", "", 200, ""}, {"GET", "bytes=0-6", 206, "console"},
	} {
		req := httptest.NewRequest(tc.method, "/dir/app.js", nil)
		if tc.rng != "" {
			req.Header.Set("Range", tc.rng)
		}
		rsp := httptest.NewRecorder()
		handler.ServeHTTP(rsp, req)
		if rsp.Code != tc.code || rsp.Body.String() != tc.body || !strings.Contains(rsp.Header().Get("Content-Type"), "javascript") {
			t.Fatalf("%+v: %d %q %v", tc, rsp.Code, rsp.Body.String(), rsp.Header())
		}
	}
	file, err := f.Open("dir/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.(io.Seeker).Seek(8, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	if err != nil || string(data) != "log('hello');" {
		t.Fatalf("%q %v", data, err)
	}
	file.Close()
	if _, err = file.Read(make([]byte, 1)); !errors.Is(err, fs.ErrClosed) {
		t.Fatal(err)
	}
}

func TestArchiveChecksum(t *testing.T) {
	fixture := archiveFixture(t, false)
	raw, err := fixture.decode(nil)
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Clone(raw)
	raw[len(raw)-8] ^= 1
	for _, raw := range [][]byte{raw, []byte("bad gzip")} {
		f, err := NewPreprocessingEmbedWithDecode(&testArchive, "test/static.tar.gz", true, func([]byte) ([]byte, error) { return raw, nil })
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.ReadDir(".")
		if err == nil {
			t.Fatal("corrupt archive accepted")
		}
	}
}
