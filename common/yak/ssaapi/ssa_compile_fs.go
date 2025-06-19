package ssaapi

import (
	"io/fs"
	"path/filepath"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

func (c *config) parseProjectWithFS(
	filesystem filesys_interface.FileSystem,
	processCallback func(float64, string, ...any),
) (*Program, error) {
	defer func() {
		if r := recover(); r != nil {
			//err = utils.Errorf("parse [%s] error %v  ", path, r)
			log.Errorf("parse project error: %s", r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	programPath := c.programPath
	prog, builder, err := c.init(filesystem)

	if err != nil {
		return nil, err
	}
	if prog.Name != "" {
		ssadb.SaveFolder(prog.Name, []string{"/"})
	}

	process := 0.0
	prog.ProcessInfof = func(s string, v ...any) {
		processCallback(
			process,
			s, v...,
		)
	}

	preHandlerTotal := 0
	handlerTotal := 0

	prog.ProcessInfof("parse project in fs: %v, path: %v", filesystem, c.info)
	prog.ProcessInfof("calculate total size of project")
	// get total size
	err = filesys.Recursive(programPath,
		filesys.WithFileSystem(filesystem),
		filesys.WithContext(c.ctx),
		filesys.WithDirStat(func(s string, fi fs.FileInfo) error {
			_, name := filesystem.PathSplit(s)
			if name == "test" || name == ".git" {
				return filesys.SkipDir
			}
			return nil
		}),
		filesys.WithFileStat(func(path string, fi fs.FileInfo) error {
			// log.Infof("calc total: %s", path)
			if fi.Size() == 0 {
				return nil
			}
			if c.excludeFile(path, fi.Name()) {
				return nil
			}
			if c.checkLanguage(path) == nil {
				handlerTotal++
			}
			if c.checkLanguagePreHandler(path) == nil {
				preHandlerTotal++
			}
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}
	if c.isStop() {
		return nil, ErrContextCancel
	}
	if (handlerTotal + preHandlerTotal) == 0 {
		return nil, ErrNoFoundCompiledFile
	}
	prog.ProcessInfof("calculate total size of project finish preHandler(len:%d) build(len:%d)", preHandlerTotal, handlerTotal)

	// pre handler  0-40%
	preHandlerNum := 0
	preHandlerProcess := func() {
		preHandlerNum++
		process = 0 + (float64(preHandlerNum)/float64(preHandlerTotal))*0.4
	}
	prog.SetPreHandler(true)
	prog.ProcessInfof("pre-handler parse project in fs: %v, path: %v", filesystem, c.info)
	filesys.Recursive(programPath,
		filesys.WithFileSystem(filesystem),
		filesys.WithContext(c.ctx),
		filesys.WithDirStat(func(s string, fi fs.FileInfo) error {
			_, name := filesystem.PathSplit(s)
			if name == "test" || name == ".git" {
				return filesys.SkipDir
			}
			return nil
		}),
		filesys.WithFileStat(func(path string, fi fs.FileInfo) (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = utils.Errorf("parse [%s] error %v  ", path, r)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			if fi.Size() == 0 {
				return nil
			}
			//check exclude_file
			if c.excludeFile(path, fi.Name()) {
				return nil
			}
			// check
			if err := c.checkLanguagePreHandler(path); err != nil {
				return nil
			}
			preHandlerProcess()
			if language := c.LanguageBuilder; language != nil {
				language.InitHandler(builder)
				language.PreHandlerProject(filesystem, builder, path)
			}
			return nil
		}),
	)
	if c.isStop() {
		return nil, ErrContextCancel
	}
	if language := c.LanguageBuilder; language != nil {
		language.AfterPreHandlerProject(builder)
	}
	prog.ProcessInfof("pre-handler parse project finish")

	process = 0.4 // 40%
	// parse project 40%-90%
	prog.ProcessInfof("parse project start")
	handlerNum := 0
	handlerProcess := func() {
		handlerNum++
		process = 0.4 + (float64(handlerNum)/float64(handlerTotal))*0.5
	}
	prog.SetPreHandler(false)
	err = ssareducer.ReducerCompile(
		programPath, // base
		ssareducer.WithFileSystem(filesystem),
		ssareducer.WithProgramName(c.ProgramName),
		ssareducer.WithEntryFiles(c.entryFile...),
		ssareducer.WithContext(c.ctx),
		ssareducer.WithStrictMode(c.strictMode),
		// ssareducer.with
		ssareducer.WithCompileMethod(func(path string, raw string) (includeFiles []string, err error) {
			defer func() {
				if r := recover(); r != nil {
					// ret = nil
					includeFiles = prog.GetIncludeFiles()
					// TODO: panic shuold be upload
					// err = utils.Errorf("parse error with panic : %v", r)
					log.Errorf("parse [%s] error %v  ", path, r)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			dir, file := filepath.Split(path)
			if c.excludeFile(dir, file) {
				return nil, nil
			}

			// check
			if err := c.checkLanguage(path); err != nil {
				log.Warnf("parse file %s error: %v", path, err)
				return nil, nil
			}
			handlerProcess()

			// build
			if err := prog.Build(path, memedit.NewMemEditor(raw), builder); err != nil {
				log.Debugf("parse %#v failed: %v", path, err)
				return nil, utils.Wrapf(err, "parse file %s error", path)
			}
			exclude := prog.GetIncludeFiles()
			if len(exclude) > 0 {
				log.Debugf("program include files: %v will not be as the entry from project", len(exclude))
			}
			return exclude, nil
		}),
	)
	if err != nil {
		return nil, utils.Wrap(err, "parse project error")
	}
	if c.isStop() {
		return nil, ErrContextCancel
	}
	process = 0.9 // %90
	prog.Finish()
	if prog.EnableDatabase { // save program
		prog.UpdateToDatabase()
	}
	total := prog.Cache.CountInstruction()
	prog.ProcessInfof("program %s finishing save cache instruction(len:%d) to database", prog.Name, total) // %90
	index := 0
	prevProcess := 0.9
	prog.Cache.SaveToDatabase(func() {
		index++
		process = 0.9 + (float64(index)/float64(total))*0.1
		if (process - prevProcess) > 0.01 { // is 91.0%/92.0%/....
			prog.ProcessInfof("Saving instructions: %d complete(total %d)", index, total)
			prevProcess = process
		}
	})
	_ = prevProcess
	return NewProgram(prog, c), nil
}
