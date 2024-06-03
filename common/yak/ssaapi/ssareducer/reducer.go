package ssareducer

import (
	"errors"
	"io/fs"

	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

var SkippedError error = utils.Error("compiling skipped")

func ReducerCompile(base string, opts ...Option) error {
	c := NewConfig(opts...)
	if c.fs == nil {
		return utils.Errorf("file system is nil")
	}

	if c.compileMethod == nil {
		return utils.Errorf("compile method is nil")
	}

	var visited = filter.NewFilter()
	defer visited.Close()

	handler := func(pathname string, fd fs.File, info fs.FileInfo) error {
		if visited.Exist(pathname) {
			return nil
		}
		if c.compileMethod == nil {
			return utils.Errorf("Compile method is nil for lib: %v", base)
		}

		results, err := c.compileMethod(pathname, fd)
		if err != nil {
			if c.stopAtCompileError {
				return err
			}
			if errors.Is(err, SkippedError) {
				return nil
			}
			log.Warnf("Compile error: %v", err)
		}
		for _, result := range results {
			visited.Insert(result)
		}
		return nil
	}

	for _, entryFile := range c.entryFiles {
		path := c.fs.Join(base, entryFile)
		info, err := c.fs.Stat(path)
		log.Infof("start to open entry file: %v", path)
		fd, err := c.fs.Open(path)
		if err != nil {
			return utils.Wrapf(err, "find entryfile failed: %v", path)
		}
		if err := handler(path, fd, info); err != nil {
			return err
		}
	}

	var fileopts []filesys.Option
	fileopts = append(fileopts, filesys.WithFileSystem(c.fs))
	fileopts = append(fileopts, filesys.WithFileStat(handler))

	err := filesys.Recursive(base, fileopts...)
	if err != nil {
		return err
	}
	return nil
}
