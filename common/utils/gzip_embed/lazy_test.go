package gzip_embed

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
)

func TestArchiveLazyResidentOnce(t *testing.T) {
	for _, first := range []string{"read", "open", "stat", "dir", "hash"} {
		t.Run(first, func(t *testing.T) {
			f := archiveFixture(t, true)
			decode := f.decode
			var calls atomic.Int32
			f.decode = func(raw []byte) ([]byte, error) { calls.Add(1); return decode(raw) }
			if calls.Load() != 0 || f.cacheFile != nil || f.cacheInfo != nil {
				t.Fatal("constructor eagerly loaded resources")
			}
			use := func(op string) error {
				switch op {
				case "read":
					data, err := f.ReadFile("dir/app.js")
					if err == nil && string(data) != "console.log('hello');" {
						t.Error("bad content")
					}
					return err
				case "open":
					file, err := f.Open("dir/app.js")
					if err == nil {
						_, err = io.Copy(io.Discard, file)
						file.Close()
					}
					return err
				case "stat":
					_, err := f.Stat("dir/app.js")
					return err
				case "dir":
					_, err := f.ReadDir("dir")
					return err
				default:
					_, err := f.GetHash()
					return err
				}
			}
			var wg sync.WaitGroup
			for i := 0; i < 32; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := use(first); err != nil {
						t.Error(err)
					}
				}()
			}
			wg.Wait()
			for _, op := range []string{"read", "open", "stat", "dir", "hash"} {
				if err := use(op); err != nil {
					t.Fatal(err)
				}
			}
			f.InvalidateHash()
			if _, err := f.GetHash(); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 || len(f.cacheFile) != 3 {
				t.Fatalf("decoded %d times, retained %d files", calls.Load(), len(f.cacheFile))
			}
			entries, err := f.ReadDir("dir")
			if err != nil {
				t.Fatal(err)
			}
			entries[0] = nil
			again, err := f.ReadDir("dir")
			if err != nil || again[0] == nil {
				t.Fatal("caller mutated cached directory")
			}
		})
	}
}

func TestArchiveLazyFailureIsRetained(t *testing.T) {
	expected := errors.New("decode failed")
	var calls atomic.Int32
	f, err := NewPreprocessingEmbedWithDecode(&testArchive, "test/static.tar.gz", true, func([]byte) ([]byte, error) { calls.Add(1); return nil, expected })
	if err != nil || calls.Load() != 0 {
		t.Fatal("constructor read archive")
	}
	for _, operation := range []func() error{
		func() error { _, e := f.ReadFile("x"); return e }, func() error { _, e := f.Open("x"); return e },
		func() error { _, e := f.Stat("."); return e }, func() error { _, e := f.ReadDir("."); return e }, func() error { _, e := f.GetHash(); return e },
	} {
		if err := operation(); !errors.Is(err, expected) {
			t.Fatalf("load failure lost: %v", err)
		}
	}
	if calls.Load() != 1 || f.cacheFile != nil || f.cacheInfo != nil {
		t.Fatal("failed load retried or published partial contents")
	}
	empty := NewEmptyPreprocessingEmbed()
	if entries, err := empty.ReadDir("."); err != nil || len(entries) != 0 {
		t.Fatalf("empty fallback: %v %v", entries, err)
	}
	if _, err := empty.ReadFile("missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestArchiveDefaultCacheAndXORCompatibility(t *testing.T) {
	legacy, err := NewPreprocessingEmbed(&testArchive, "test/static.tar.gz")
	if err != nil || !legacy.EnableCache || legacy.cacheFile != nil {
		t.Fatal("default must be lazy and cached")
	}
	want, err := legacy.ReadFile("1.txt")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := testArchive.ReadFile("test/static.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	encoded := bytes.Clone(raw)
	xorInPlace(encoded, []byte(DefaultXORKey))
	if bytes.Equal(encoded, raw) {
		t.Fatal("default XOR disabled")
	}
	decoded, err := decodeDefaultArchive(bytes.Clone(encoded))
	if err != nil || !bytes.Equal(decoded, raw) {
		t.Fatal("default key mismatch")
	}
	unchanged, err := decodeDefaultArchive(bytes.Clone(raw))
	if err != nil || !bytes.Equal(unchanged, raw) {
		t.Fatal("legacy gzip changed")
	}
	f, err := NewPreprocessingEmbedWithDecode(&testArchive, "test/static.tar.gz", true, func([]byte) ([]byte, error) { return decodeDefaultArchive(bytes.Clone(encoded)) })
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.ReadFile("1.txt")
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("default XOR load failed")
	}
	key := []byte("custom-key")
	custom, err := NewPreprocessingEmbedWithXORKey(&testArchive, "test/static.tar.gz", true, key)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Clone(raw)
	xorInPlace(payload, key)
	key[0] = 'X'
	originalDecode := custom.decode
	custom.decode = func([]byte) ([]byte, error) { return originalDecode(bytes.Clone(payload)) }
	got, err = custom.ReadFile("1.txt")
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("explicit key compatibility failed")
	}
}

var benchFS *PreprocessingEmbed

func BenchmarkArchiveConstruction(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchFS, _ = NewPreprocessingEmbed(&testArchive, "test/static.tar.gz")
	}
}
func BenchmarkArchiveWarmRead(b *testing.B) {
	f, err := NewPreprocessingEmbed(&testArchive, "test/static.tar.gz")
	if err != nil {
		b.Fatal(err)
	}
	if _, err = f.ReadFile("1.txt"); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = f.ReadFile("1.txt"); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkArchiveXOR1MiB(b *testing.B) {
	data := make([]byte, 1<<20)
	key := []byte(DefaultXORKey)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		xorInPlace(data, key)
	}
}
