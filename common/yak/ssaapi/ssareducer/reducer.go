package ssareducer

import (
	"errors"
	"io"
	"io/fs"
	"strings"

	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
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

	handler := func(path string) error {

		fd, err := c.fs.Open(path)
		if err != nil {
			return utils.Wrapf(err, "c.fs.Open(%#v) failed", path)
		}
		// , err := fd.Read()
		data, err := io.ReadAll(fd)
		if err != nil {
			return utils.Wrapf(err, "io.ReadAll(%#v) failed: %v", path, err)
		}
		content := utils.UnsafeBytesToString(data)
		defer func() {
			fd.Close()
		}()

		if c.compileMethod == nil {
			return utils.Errorf("Compile method is nil for lib: %v", base)
		}

		results, err := c.compileMethod(path, content)
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
		if err != nil {
			return utils.Wrapf(err, "find entryfile failed: %v", path)
		}
		_ = info
		if err := handler(path); err != nil {
			return err
		}
	}

	var fileopts []filesys.Option
	fileopts = append(fileopts, filesys.WithFileSystem(c.fs))
	fileopts = append(fileopts, filesys.WithStat(func(isDir bool, path string, fi fs.FileInfo) error {
		if visited.Exist(path) {
			return nil
		}
		if isDir && c.ProgramName != "" {
			folder, name := c.fs.PathSplit(path)
			folders := []string{c.ProgramName}
			folders = append(folders,
				strings.Split(folder, string(c.fs.GetSeparators()))...,
			)
			ssadb.SaveFolder(name, folders)
		} else {
			handler(path)
		}
		return nil
	}))

	err := filesys.Recursive(base, fileopts...)
	if err != nil {
		return err
	}
	return nil
}
