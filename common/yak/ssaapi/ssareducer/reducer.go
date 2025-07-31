package ssareducer

import (
	"errors"
	"io"
	"io/fs"
	"strings"

	"github.com/yaklang/yaklang/common/filter"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

var SkippedError error = utils.Error("compiling skipped")

func ReducerCompile(base string, opts ...Option) error {
	c := NewConfig(opts...)
	if c.fs == nil {
		return utils.Errorf("file system is nil")
	}

	cancel := c.GetCancelFunc()

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
		defer func() {
			fd.Close()
		}()
		data, err := io.ReadAll(fd)
		if err != nil {
			return utils.Wrapf(err, "io.ReadAll(%#v) failed: %v", path, err)
		}
		if len(data) == 0 {
			log.Errorf("file %s is empty", path)
			return nil
		}
		content := utils.UnsafeBytesToString(data)

		if c.compileMethod == nil {
			return utils.Errorf("Compile method is nil for lib: %v", base)
		}

		results, err := c.compileMethod(path, content)
		if err != nil {
			if errors.Is(err, SkippedError) {
				return nil
			}
			if c.stopAtCompileError {
				cancel()
			}
			return err
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
		if !isDir {
			// file
			if err := handler(path); err != nil {
				return err
			}
			return nil
		}
		folder, name := c.fs.PathSplit(path)
		// if test or .git, skip
		if name == "test" || name == ".git" || name == ".svn" || name == ".vscode" || name == ".idea" {
			return filesys.SkipDir
		}
		// if have Database, save folder
		if c.ProgramName != "" {
			folders := []string{c.ProgramName}
			folders = append(folders,
				strings.Split(folder, string(c.fs.GetSeparators()))...,
			)
			// ssadb.SaveFolder(name, folders)
		}
		return nil
	}))
	fileopts = append(fileopts, filesys.WithContext(c.ctx))
	err := filesys.Recursive(base, fileopts...)
	if err != nil {
		return err
	}
	return nil
}
