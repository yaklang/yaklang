package ssaapi

import (
	"fmt"
	"io/fs"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

func (c *config) parseProject() (Programs, error) {

	defer func() {
		if r := recover(); r != nil {
			// err = utils.Errorf("parse [%s] error %v  ", path, r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	if c.reCompile {
		ssadb.DeleteProgram(ssadb.GetDB(), c.ProgramName)
		ssadb.DeleteSSAProgram(c.ProgramName)
	}
	if c.databasePath != "" {
		consts.SetSSADataBasePath(c.databasePath)
	}
	programPath := c.programPath
	prog, builder, err := c.init()

	if err != nil {
		return nil, err
	}
	if prog.Name != "" {
		ssadb.SaveFolder(prog.Name, []string{"/"})
	}

	totalProcess := 0
	handledProcess := 0
	prog.ProcessInfof = func(s string, v ...any) {
		msg := fmt.Sprintf(s, v...)
		// handled := len(prog.FileList)
		if c.process != nil {
			c.process(msg, float64(handledProcess)/float64(totalProcess))
		} else {
			log.Info(msg)
		}
	}
	preHandlerSize := 0
	parseSize := 0

	// get total size
	filesys.Recursive(programPath,
		filesys.WithFileSystem(c.fs),
		filesys.WithContext(c.ctx),
		filesys.WithDirStat(func(s string, fi fs.FileInfo) error {
			_, name := c.fs.PathSplit(s)
			if name == "test" || name == ".git" {
				return filesys.SkipDir
			}
			return nil
		}),
		filesys.WithFileStat(func(path string, fi fs.FileInfo) error {
			if fi.Size() == 0 {
				return nil
			}
			if c.checkLanguage(path) == nil {
				parseSize++
			}
			if c.checkLanguagePreHandler(path) == nil {
				preHandlerSize++
			}
			// log.Infof("nomatch when calc total: %s", path)
			return nil
		}),
	)
	if c.isStop() {
		return nil, utils.Errorf("parse project stop")
	}
	if (parseSize + preHandlerSize) == 0 {
		return nil, utils.Errorf("no file can compile with language[%s]", c.language)
	}
	totalProcess = parseSize + preHandlerSize + 1

	// pre handler
	prog.SetPreHandler(true)
	prog.ProcessInfof("pre-handler parse project in fs: %v, path: %v", c.fs, programPath)
	filesys.Recursive(programPath,
		filesys.WithFileSystem(c.fs),
		filesys.WithContext(c.ctx),
		filesys.WithDirStat(func(s string, fi fs.FileInfo) error {
			_, name := c.fs.PathSplit(s)
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
			// check
			if err := c.checkLanguagePreHandler(path); err != nil {
				return nil
			}
			handledProcess++
			if language := c.LanguageBuilder; language != nil {
				language.InitHandler(builder)
				language.PreHandlerProject(c.fs, builder, path)
			}
			return nil
		}),
	)
	if c.isStop() {
		return nil, utils.Errorf("parse project stop")
	}
	prog.ProcessInfof("pre-handler parse project finish")
	handledProcess = preHandlerSize // finish pre-handler 50%

	// parse project
	prog.ProcessInfof("parse project start")
	prog.SetPreHandler(false)
	err = ssareducer.ReducerCompile(
		programPath, // base
		ssareducer.WithFileSystem(c.fs),
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

			// check
			if err := c.checkLanguage(path); err != nil {
				log.Warnf("parse file %s error: %v", path, err)
				return nil, nil
			}
			handledProcess++

			// build
			if err := prog.Build(path, memedit.NewMemEditor(raw), builder); err != nil {
				log.Debugf("parse %#v failed: %v", path, err)
				return nil, utils.Wrapf(err, "parse file %s error", path)
			}
			exclude := prog.GetIncludeFiles()
			if len(exclude) > 0 {
				log.Infof("program include files: %v will not be as the entry from project", len(exclude))
			}
			return exclude, nil
		}),
	)
	if err != nil {
		return nil, utils.Wrap(err, "parse project error")
	}
	if c.isStop() {
		return nil, utils.Errorf("parse project stop")
	}
	handledProcess = preHandlerSize + parseSize
	prog.ProcessInfof("program %s finishing", prog.Name) // %99
	prog.Finish()
	var progs = []*Program{NewProgram(prog, c)}
	c.SaveProfile()
	handledProcess = preHandlerSize + parseSize + 1
	prog.ProcessInfof("program %s finish", prog.Name) // %100
	return progs, nil
}
