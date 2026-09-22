package testcheck

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"
)

func archiveFixture(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, name := range names {
		h := &zip.FileHeader{Name: name, Method: zip.Store}
		h.SetMode(0644)
		w, err := z.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte("fixed corpus bytes")); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
func TestFixtureMemoryFS(t *testing.T) {
	data := archiveFixture(t, "nested/input.lock", "second.lock")
	f, err := LoadFixtures(fstest.MapFS{"testdata/cargo_lock_le_v3_corpus_v1.zip": {Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	if err := fstest.TestFS(f, "testdata/nested/input.lock", "testdata/second.lock"); err != nil {
		t.Fatal(err)
	}
	a, err := f.ReadFile("testdata/nested/input.lock")
	if err != nil {
		t.Fatal(err)
	}
	a[0] = 'X'
	b, err := f.ReadFile("testdata/nested/input.lock")
	if err != nil || string(b) != "fixed corpus bytes" {
		t.Fatalf("shared mutation: %q %v", b, err)
	}
	reader, err := f.OpenReader("testdata/nested/input.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err = reader.Seek(6, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	if err != nil || string(body) != "corpus bytes" {
		t.Fatalf("seek: %q %v", body, err)
	}
	p := make([]byte, 5)
	if _, err = reader.ReadAt(p, 0); err != nil || string(p) != "fixed" {
		t.Fatalf("readat: %q %v", p, err)
	}
	sub := f.DirFS("testdata/nested")
	body, err = fs.ReadFile(sub, "input.lock")
	if err != nil || !bytes.Equal(body, b) {
		t.Fatal("sub-FS changed input", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := f.ReadFile("testdata/second.lock")
			if e != nil || string(v) != "fixed corpus bytes" {
				t.Errorf("concurrent read: %q %v", v, e)
			}
		}()
	}
	wg.Wait()
	if _, err = f.ReadFile("../input.lock"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("invalid path: %v", err)
	}
}
func TestFixtureArchiveFailures(t *testing.T) {
	for _, names := range [][]string{{"../escape"}, {"a\\b"}, {"/absolute"}, {"same", "same"}} {
		if _, err := LoadFixtures(fstest.MapFS{"testdata/corpus_v1.zip": {Data: archiveFixture(t, names...)}}); err == nil {
			t.Fatalf("accepted unsafe/duplicate entries: %v", names)
		}
	}
	if _, err := LoadFixtures(fstest.MapFS{"testdata/corpus_v1.zip": {Data: []byte("truncated")}}); err == nil {
		t.Fatal("accepted malformed ZIP")
	}
	data := archiveFixture(t, "input")
	at := bytes.Index(data, []byte("fixed corpus bytes"))
	if at < 0 {
		t.Fatal("missing stored payload")
	}
	data[at] = 'X'
	f, err := LoadFixtures(fstest.MapFS{"testdata/corpus_v1.zip": {Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.ReadFile("testdata/input"); !errors.Is(err, zip.ErrChecksum) {
		t.Fatalf("CRC not verified: %v", err)
	}
}
