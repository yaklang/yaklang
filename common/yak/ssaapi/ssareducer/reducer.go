package ssareducer

import (
	"io/fs"
	"path/filepath"

	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

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

	if len(c.entryFiles) <= 0 {
		return utils.Error("entry-files is not set for base, try to auto select (TBD)")
	}

	for _, entryFile := range c.entryFiles {
		info, err := c.fs.Stat(entryFile)
		if err != nil {
			var d string
			d, entryFile = filepath.Split(entryFile)
			log.Warnf("directory is repeated... split dir [%v] and filename, and use %v", d, entryFile)
		} else {
			if info.IsDir() {
				log.Warnf("entry [%v] cannot be as directory...", entryFile)
				continue
			}
		}

		fd, err := c.fs.Open(entryFile)
		if err != nil {
			return utils.Wrapf(err, "find entryfile failed: %v", entryFile)
		}
		results, err := c.compileMethod(entryFile, fd)
		if err != nil {
			return err
		}
		for _, result := range results {
			visited.Insert(result)
		}
	}

	var fileopts []filesys.Option
	fileopts = append(fileopts, filesys.WithFileSystem(c.fs))

	fileopts = append(fileopts,
		filesys.WithFileStat(func(pathname string, fd fs.File, info fs.FileInfo) error {
			if !c.filter(pathname) {
				return nil
			}

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
				log.Warnf("Compile error: %v", err)
			}
			for _, result := range results {
				visited.Insert(result)
			}
			return nil
		}),
	)

	err := filesys.Recursive(base, fileopts...)
	if err != nil {
		return err
	}
	return nil
}
