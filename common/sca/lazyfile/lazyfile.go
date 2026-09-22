package lazyfile

import (
	"context"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"io"
	"io/fs"
	"sync"

	"fmt"
	"os"
)

type LazyFile struct {
	ctx context.Context

	fs.File
	io.Seeker
	io.ReaderAt

	fileName string
	openOnce *sync.Once
	finalErr error

	finalOpen func() (file fs.File, err error)
}

func LazyOpenStreamByFilePath(fsIns fs.FS, name string) *LazyFile {
	if fsIns == nil {
		fsIns = localFS{}
	}
	lf := &LazyFile{
		openOnce: new(sync.Once),
	}
	lf.fileName = name

	lf.finalOpen = func() (file fs.File, err error) {
		if lf.finalErr != nil {
			return nil, lf.finalErr
		}

		f, err := fsIns.Open(name)
		if err != nil {
			lf.finalErr = err
			return nil, err
		}
		lf.File = f
		return f, nil
	}
	return lf
}

func LazyOpenStreamByFile(f fs.FS, rf fs.File) *LazyFile {
	var name string
	info, _ := rf.Stat()
	if info != nil {
		name = info.Name()
	}

	if nameGetter, ok := rf.(interface{ Name() string }); ok {
		name = nameGetter.Name()
	}

	return LazyOpenStreamByFilePath(f, name)
}

func (f *LazyFile) lazyOpen() error {
	if err := f.Context().Err(); err != nil {
		return err
	}
	f.openOnce.Do(func() {
		_, f.finalErr = f.finalOpen()
	})
	return f.finalErr
}

func (f *LazyFile) ReadAt(b []byte, off int64) (n int, err error) {
	err = f.lazyOpen()
	if err != nil {
		return 0, err
	}

	if ins, ok := f.File.(interface {
		ReadAt([]byte, int64) (int, error)
	}); ok {
		n, e := ins.ReadAt(b, off)
		if err := budget.From(f.Context()).Read(int64(n)); err != nil {
			return n, err
		}
		return n, e
	}
	return 0, fmt.Errorf("ReadAt not supported: %w", fs.ErrInvalid)
}

func (f *LazyFile) Read(b []byte) (int, error) {
	err := f.lazyOpen()
	if err != nil {
		return 0, err
	}
	n, e := f.File.Read(b)
	if err := budget.From(f.Context()).Read(int64(n)); err != nil {
		return n, err
	}
	return n, e
}

func (f *LazyFile) Stat() (fs.FileInfo, error) {
	err := f.lazyOpen()
	if err != nil {
		return nil, err
	}
	return f.File.Stat()
}

//func (f *LazyFile) Write(b []byte) (int, error) {
//	err := f.lazyOpen()
//	if err != nil {
//		return 0, err
//	}
//	return f.File.Write(b)
//}

func (f *LazyFile) Close() error {
	if f.File == nil {
		return nil
	}

	err := f.finalErr
	if err != nil {
		return err
	}
	if f.File != nil {
		return f.File.Close()
	}
	return nil
}

func (f *LazyFile) Name() string {
	return f.fileName
}

func (f *LazyFile) Seek(offset int64, whence int) (int64, error) {
	err := f.lazyOpen()
	if err != nil {
		return 0, err
	}

	if ins, ok := f.File.(interface {
		Seek(int64, int) (int64, error)
	}); ok {
		return ins.Seek(offset, whence)
	}
	return 0, fmt.Errorf("Seek not supported: %w", fs.ErrInvalid)
}

type localFS struct{}

func (localFS) Open(name string) (fs.File, error) { return os.Open(name) }

func (f *LazyFile) SetContext(ctx context.Context) { f.ctx = ctx }
func (f *LazyFile) Context() context.Context {
	if f.ctx == nil {
		return context.Background()
	}
	return f.ctx
}
