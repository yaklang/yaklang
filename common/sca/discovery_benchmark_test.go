package sca

import (
	"fmt"
	"io"
	"io/fs"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/analyzer"
	"github.com/yaklang/yaklang/common/sca/internal/fsio"
)

type countedSnapshot struct {
	fs.FS
	opens, reads atomic.Int64
}

func (s *countedSnapshot) Open(name string) (fs.File, error) {
	f, err := s.FS.Open(name)
	if err != nil {
		return nil, err
	}
	s.opens.Add(1)
	return &countedFile{File: f, owner: s}, nil
}
func (s *countedSnapshot) ReadDir(name string) ([]fs.DirEntry, error) { return fs.ReadDir(s.FS, name) }
func (s *countedSnapshot) Stat(name string) (fs.FileInfo, error)      { return fs.Stat(s.FS, name) }

type countedFile struct {
	fs.File
	owner *countedSnapshot
}

func (f *countedFile) Read(b []byte) (int, error) {
	n, err := f.File.Read(b)
	f.owner.reads.Add(int64(n))
	return n, err
}

func (f *countedFile) ReadAt(b []byte, off int64) (int, error) {
	r, ok := f.File.(io.ReaderAt)
	if !ok {
		return 0, fs.ErrInvalid
	}
	n, err := r.ReadAt(b, off)
	f.owner.reads.Add(int64(n))
	return n, err
}
func (f *countedFile) Seek(off int64, whence int) (int64, error) {
	r, ok := f.File.(io.Seeker)
	if !ok {
		return 0, fs.ErrInvalid
	}
	return r.Seek(off, whence)
}

// The same readonly input adapter and benchmark can run in the preserved old
// checkout. Only successful file opens and actual bytes read are counted; these
// metrics do not claim to trace host syscalls or temporary files.
func BenchmarkSparseDiscovery(b *testing.B) {
	for _, n := range []int{100, 1000, 10000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			input := fstest.MapFS{"go.mod": {Data: []byte("module example.test/app\nrequire example.test/a v1.2.3\n")}}
			for i := 0; i < n; i++ {
				input[fmt.Sprintf("file-%06d.txt", i)] = &fstest.MapFile{Data: []byte("not a dependency file\n")}
			}
			s := &countedSnapshot{FS: input}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pkgs, err := ScanFilesystem(fsio.New(s), _withConcurrent(1), _withAnalayzers(analyzer.TypGoMod))
				if err != nil || len(pkgs) != 1 || pkgs[0].Name != "example.test/a" || pkgs[0].Version != "1.2.3" {
					b.Fatalf("incomparable inventory: %+v, %v", pkgs, err)
				}
			}
			b.ReportMetric(float64(s.opens.Load())/float64(b.N), "opens/op")
			b.ReportMetric(float64(s.reads.Load())/float64(b.N), "readbytes/op")
		})
	}
}

func TestTextOnlyDiscoveryDoesNotReadUnrelatedHeaders(t *testing.T) {
	mod := []byte("module example.test/app\nrequire example.test/a v1.2.3\n")
	input := fstest.MapFS{"go.mod": {Data: mod}}
	for i := 0; i < 1000; i++ {
		input[fmt.Sprintf("file-%04d.txt", i)] = &fstest.MapFile{Data: []byte("irrelevant")}
	}
	s := &countedSnapshot{FS: input}
	pkgs, err := ScanFilesystem(fsio.New(s), _withConcurrent(1), _withAnalayzers(analyzer.TypGoMod))
	if err != nil || len(pkgs) != 1 || pkgs[0].Name != "example.test/a" {
		t.Fatalf("%+v %v", pkgs, err)
	}
	if s.opens.Load() != 1 || s.reads.Load() != int64(len(mod)) {
		t.Fatalf("unrelated material read: opens=%d bytes=%d", s.opens.Load(), s.reads.Load())
	}
}
