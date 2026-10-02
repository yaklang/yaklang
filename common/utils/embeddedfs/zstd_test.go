package embeddedfs

import (
	"archive/tar"
	"bytes"
	"errors"
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/klauspost/compress/zstd"
)

func testArchive(t *testing.T, names ...string) string {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, name := range names {
		b := []byte("contents of " + name)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0444, Size: int64(len(b))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	return string(enc.EncodeAll(buf.Bytes(), nil))
}

func TestLazyArchiveFilesystem(t *testing.T) {
	f := NewZstd(testArchive(t, "root.yaml", "application/http.yaml", "application/empty.yaml"), 1<<20)
	if f.entries != nil || f.err != nil {
		t.Fatal("constructor loaded archive")
	}
	if _, err := f.Open("../escape"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("invalid name: %v", err)
	}
	if f.entries != nil {
		t.Fatal("invalid path initialized archive")
	}
	if err := fstest.TestFS(f, "root.yaml", "application/http.yaml", "application/empty.yaml"); err != nil {
		t.Fatal(err)
	}
	first, err := f.ReadFile("root.yaml")
	if err != nil {
		t.Fatal(err)
	}
	first[0] = 'X'
	second, err := fs.ReadFile(f, "root.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != "contents of root.yaml" {
		t.Fatal("ReadFile exposed shared bytes")
	}
	list, err := f.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	list[0] = nil
	list, err = f.ReadDir(".")
	if err != nil || list[0] == nil {
		t.Fatal("ReadDir exposed shared slice")
	}
}

func TestConcurrentFirstAccess(t *testing.T) {
	f := NewZstd(testArchive(t, "application/http.yaml"), 1<<20)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				b, err := f.ReadFile("application/http.yaml")
				if err != nil || string(b) != "contents of application/http.yaml" {
					t.Errorf("read: %q, %v", b, err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestArchiveErrorsAndLimits(t *testing.T) {
	good := testArchive(t, "a.yaml")
	broken := []byte(good)
	broken[len(broken)-1] ^= 0xff
	for name, f := range map[string]*FS{
		"bad frame":         NewZstd("not zstd", 1<<20),
		"checksum":          NewZstd(string(broken), 1<<20),
		"truncated":         NewZstd(good[:len(good)-2], 1<<20),
		"size limit":        NewZstd(good, 16),
		"zero limit":        NewZstd(good, 0),
		"duplicate":         NewZstd(testArchive(t, "a.yaml", "a.yaml"), 1<<20),
		"traversal":         NewZstd(testArchive(t, "../a.yaml"), 1<<20),
		"absolute":          NewZstd(testArchive(t, "/a.yaml"), 1<<20),
		"file as directory": NewZstd(testArchive(t, "a", "a/b"), 1<<20),
		"directory as file": NewZstd(testArchive(t, "a/b", "a"), 1<<20),
	} {
		t.Run(name, func(t *testing.T) {
			_, first := f.Open("a.yaml")
			_, second := f.Open("a.yaml")
			if first == nil || second == nil || first.Error() != second.Error() {
				t.Fatalf("non-deterministic error: %v / %v", first, second)
			}
			if f.entries != nil {
				t.Fatal("failed archive published partial index")
			}
		})
	}
}
