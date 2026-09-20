package embedfs

import (
	"bytes"
	"compress/gzip"
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

func compressed(t *testing.T, data string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(data)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
func fixture(t *testing.T) FS {
	return New(fstest.MapFS{
		"dir/app.js" + Suffix: {Data: compressed(t, "console.log('hello');"), Mode: 0444},
		"dir/plain.txt":       {Data: []byte("plain"), Mode: 0444},
		"dir/original.gz":     {Data: compressed(t, "archive"), Mode: 0444},
		"dir/z.txt" + Suffix:  {Data: compressed(t, "last"), Mode: 0444},
	})
}
func TestFilesystemContract(t *testing.T) {
	f := fixture(t)
	if err := fstest.TestFS(f, "dir/app.js", "dir/plain.txt", "dir/original.gz", "dir/z.txt"); err != nil {
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
	entries, err := f.ReadDir("dir")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		data, err := f.ReadFile("dir/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() != int64(len(data)) || info.Name() != entry.Name() {
			t.Fatalf("invalid metadata: %v", info)
		}
	}
	for _, name := range []string{"../dir/app.js", "/dir/app.js", "dir/../dir/app.js", "dir/app.js" + Suffix, ""} {
		for _, op := range []func(string) error{
			func(n string) error { _, e := f.Open(n); return e },
			func(n string) error { _, e := f.Stat(n); return e },
			func(n string) error { _, e := f.ReadDir(n); return e },
		} {
			if err := op(name); !errors.Is(err, fs.ErrInvalid) {
				t.Fatalf("%q: %v", name, err)
			}
		}
	}
	_, err = f.Open("missing")
	var pe *fs.PathError
	if !errors.Is(err, fs.ErrNotExist) || !errors.As(err, &pe) || pe.Path != "missing" {
		t.Fatal(err)
	}
}
func TestIndependentReadersAndChecksum(t *testing.T) {
	f := fixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
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
	if _, err := file.Read(make([]byte, 1)); !errors.Is(err, fs.ErrClosed) {
		t.Fatal(err)
	}
	raw := compressed(t, "hello")
	raw[len(raw)-8] ^= 1
	for _, raw := range [][]byte{raw, []byte("bad gzip")} {
		_, err := New(fstest.MapFS{"bad" + Suffix: {Data: raw}}).ReadFile("bad")
		if err == nil {
			t.Fatal("corrupt gzip accepted")
		}
	}
}
func TestHTTPFilesystem(t *testing.T) {
	handler := http.FileServer(http.FS(fixture(t)))
	for _, tc := range []struct {
		method, path, rng string
		code              int
		body              string
	}{
		{"GET", "/dir/app.js", "", 200, "console.log('hello');"},
		{"HEAD", "/dir/app.js", "", 200, ""},
		{"GET", "/dir/app.js", "bytes=0-6", 206, "console"},
		{"GET", "/missing", "", 404, "404 page not found\n"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.rng != "" {
			req.Header.Set("Range", tc.rng)
		}
		rsp := httptest.NewRecorder()
		handler.ServeHTTP(rsp, req)
		if rsp.Code != tc.code || rsp.Body.String() != tc.body {
			t.Fatalf("%+v: %d %q", tc, rsp.Code, rsp.Body.String())
		}
		if strings.HasSuffix(tc.path, ".js") && !strings.Contains(rsp.Header().Get("Content-Type"), "javascript") {
			t.Fatal(rsp.Header())
		}
	}
}
