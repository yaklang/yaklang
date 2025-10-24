package ssaapi

import (
	"errors"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
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

func ParseProject(opts ...Option) (prog Programs, err error) {
	config, err := DefaultConfig(opts...)
	if err != nil {
		return nil, err
	}
	defer func() {
		ssaprofile.ShowCacheCost()
	}()
	f1 := func() {
		prog, err = config.parseProject()
	}
	ssaprofile.ProfileAdd(true, "ssaapi.ParseProject", f1)
	return
}

func (c *Config) parseProject() (Programs, error) {

	if c.GetCompileReCompile() {
		c.Processf(0, "recompile project, delete old data...")
		ssadb.DeleteProgramIrCode(ssadb.GetDB(), c.ProgramName)
		c.Processf(0, "recompile project, delete old data finish")
	}

	c.Processf(0, "recompile project, start compile")
	if c.GetCompilePeepholeSize() != 0 {
		// peephole compile
		if progs, err := c.peephole(); err != nil {
			return nil, err
		} else {
			SaveConfig(c, nil)
			c.Processf(1, "programs finish")
			return progs, nil
		}
	} else {
		// normal compile
		if prog, err := c.parseProjectWithFS(c.fs, func(f float64, s string, a ...any) {
			c.Processf(f*0.99, s, a...)
		}); err != nil {
			return nil, err
		} else {
			SaveConfig(c, prog)
			c.Processf(1, "program %s finish", prog.GetProgramName())
			return Programs{prog}, nil
		}
	}
}

func (c *Config) peephole() (Programs, error) {

	originFs := c.fs
	if originFs == nil {
		return nil, utils.Errorf("need set filesystem")
	}

	progs := make(Programs, 0)
	var errs error

	filesys.Peephole(originFs,
		filesys.WithPeepholeSize(c.GetCompilePeepholeSize()),
		filesys.WithPeepholeContext(c.ctx),
		filesys.WithPeepholeCallback(func(count, totalCount int, system filesys_interface.FileSystem) {
			totalCount = totalCount + 1
			baseProcess := float64(count-1) / float64(totalCount)
			prog, err := c.parseProjectWithFS(system, func(f float64, s string, a ...any) {
				c.Processf(baseProcess+f/float64(totalCount), s, a)
			})
			process := float64(count) / float64(totalCount) // max is 99%
			c.Processf(process, "finish peephole filesystem")
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
