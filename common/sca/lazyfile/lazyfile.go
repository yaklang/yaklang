package lazyfile

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"io"
	"io/fs"
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

type LazyFile struct {
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
		fsIns = filesys.NewLocalFs()
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

	if ins, ok := f.File.(interface {
		ReadAt([]byte, int64) (int, error)
	}); ok {
		return ins.ReadAt(b, off)
	}
	return 0, utils.Wrap(fs.ErrInvalid, "ReadAt not supported")
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
	return 0, utils.Wrap(fs.ErrInvalid, "Seek not supported")
}
