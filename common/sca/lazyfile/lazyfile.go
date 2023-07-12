package lazyfile

import (
	"io"
	"io/fs"
	"os"
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

type LazyFile struct {
	*os.File

	fileName string
	openOnce *sync.Once
	finalErr error

	finalOpen func() (file *os.File, err error)
}

func LazyOpenReadCloserFile(name string, flag int, perm os.FileMode) io.ReadCloser {
	lf := &LazyFile{
		openOnce: new(sync.Once),
	}
	lf.fileName = name

	lf.finalOpen = func() (file *os.File, err error) {
		if lf.finalErr != nil {
			return nil, lf.finalErr
		}

		f, err := os.OpenFile(name, flag, perm)
		if err != nil {
			lf.finalErr = err
			return nil, err
		}
		lf.File = f
		return f, nil
	}
	return lf
}

func LazyOpenStreamByFilePath(name string) *LazyFile {
	lf := &LazyFile{
		openOnce: new(sync.Once),
	}
	lf.fileName = name

	lf.finalOpen = func() (file *os.File, err error) {
		if lf.finalErr != nil {
			return nil, lf.finalErr
		}

		f, err := os.OpenFile(name, os.O_RDWR, os.ModePerm)
		if err != nil {
			lf.finalErr = err
			return nil, err
		}
		lf.File = f
		return f, nil
	}
	return lf
}

func LazyOpenStreamByFile(rf *os.File) *LazyFile {
	return LazyOpenStreamByFilePath(rf.Name())
}

func (f *LazyFile) lazyOpen() error {
	f.openOnce.Do(func() {
		_, err := f.finalOpen()
		if err != nil {
			log.Errorf("lazyOpen failed: %v", err)
		}
	})
	return f.finalErr
}

func (f *LazyFile) ReadAt(b []byte, off int64) (n int, err error) {
	err = f.lazyOpen()
	if err != nil {
		return 0, err
	}
	return f.File.ReadAt(b, off)
}

func (f *LazyFile) Read(b []byte) (int, error) {
	err := f.lazyOpen()
	if err != nil {
		return 0, err
	}
	return f.File.Read(b)
}

func (f *LazyFile) Stat() (fs.FileInfo, error) {
	err := f.lazyOpen()
	if err != nil {
		return nil, err
	}
	return f.File.Stat()
}

func (f *LazyFile) Write(b []byte) (int, error) {
	err := f.lazyOpen()
	if err != nil {
		return 0, err
	}
	return f.File.Write(b)
}

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
