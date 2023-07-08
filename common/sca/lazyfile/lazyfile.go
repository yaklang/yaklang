package lazyfile

import (
	"github.com/yaklang/yaklang/common/log"
	"io"
	"os"
	"sync"
)

type lazyFile struct {
	fileName  string
	finalOpen func() (file *os.File, err error)
	finalErr  error
	*os.File
	openOnce *sync.Once
}

func LazyOpenReadCloserFile(name string, flag int, perm os.FileMode) io.ReadCloser {
	lf := &lazyFile{
		openOnce: new(sync.Once),
	}
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

func LazyOpenStreamByFile(name string, flag int, perm os.FileMode) io.ReadWriteCloser {
	lf := &lazyFile{
		openOnce: new(sync.Once),
	}
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

func (f *lazyFile) lazyOpen() error {
	f.openOnce.Do(func() {
		_, err := f.finalOpen()
		if err != nil {
			log.Errorf("lazyOpen failed: %v", err)
		}
	})
	return f.finalErr
}

func (f *lazyFile) Read(b []byte) (int, error) {
	err := f.lazyOpen()
	if err != nil {
		return 0, err
	}
	return f.File.Read(b)
}

func (f *lazyFile) Write(b []byte) (int, error) {
	err := f.lazyOpen()
	if err != nil {
		return 0, err
	}
	return f.File.Write(b)
}

func (f *lazyFile) Close() error {
	if f.File == nil {
		log.Warnf("")
		return nil
	}

	err := f.lazyOpen()
	if err != nil {
		return err
	}
	if f.File != nil {
		return f.File.Close()
	}
	return nil
}
