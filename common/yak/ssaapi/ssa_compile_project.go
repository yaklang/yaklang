package ssaapi

import (
	"errors"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func ParseProjectFromPath(path string, opts ...Option) (Programs, error) {
	if path != "" {
		opts = append(opts, WithLocalFs(path))
	}
	return ParseProject(opts...)
}

func ParseProjectWithFS(fs fi.FileSystem, opts ...Option) (Programs, error) {
	opts = append(opts, WithFileSystem(fs))
	return ParseProject(opts...)
}

func PeepholeCompile(fs fi.FileSystem, size int, opts ...Option) (Programs, error) {
	opts = append(opts, WithFileSystem(fs), WithPeepholeSize(size))
	return ParseProject(opts...)
}

func ParseProject(opts ...Option) (Programs, error) {
	config, err := defaultConfig(opts...)
	if err != nil {
		return nil, err
	}
	return config.parseProject()

}

func (c *config) parseProject() (Programs, error) {
	if c.reCompile {
		ssadb.DeleteProgram(ssadb.GetDB(), c.ProgramName)
		ssadb.DeleteSSAProgram(c.ProgramName)
	}
	if c.databasePath != "" {
		consts.SetSSADataBasePath(c.databasePath)
	}

	if c.peepholeSize != 0 {
		// peephole compile
		if progs, err := c.peephole(); err != nil {
			return nil, err
		} else {
			return progs, nil
		}
	} else {
		// normal compile
		if prog, err := c.parseProjectWithFS(c.fs, c.Processf); err != nil {
			return nil, err
		} else {
			return Programs{prog}, nil
		}
	}
}

func (c *config) peephole() (Programs, error) {

	originFs := c.fs
	if originFs == nil {
		return nil, utils.Errorf("need set filesystem")
	}

	progs := make(Programs, 0)
	var errs error

	//TODO: calculate process in peephole compile
	process := func(f float64, s string, a ...any) {
		c.Processf(f, s, a...)
	}

	filesys.Peephole(originFs,
		filesys.WithPeepholeSize(c.peepholeSize),
		filesys.WithPeepholeCallback(func(system filesys_interface.FileSystem) {
			prog, err := c.parseProjectWithFS(system, process)
			// if no err just append and return
			if err == nil {
				progs = append(progs, prog)
				return
			}

			// check error
			if errors.Is(err, ErrNoFoundCompiledFile) {
				return
			}
			errs = utils.JoinErrors(errs, err)
		}),
	)
	return progs, errs
}
