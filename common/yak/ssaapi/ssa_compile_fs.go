package ssaapi

import (
	"io/fs"
	"sync"
	"time"

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

	var calculateTime, preHandlerTime, parseTime, saveTime time.Duration
	defer func() {
		log.Errorf("calculate time: %v", calculateTime)
		log.Errorf("pre-handler time: %v", preHandlerTime)
		log.Errorf("parse time: %v", parseTime)
		log.Errorf("save time: %v", saveTime)
	}()

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
		go processCallback(
			process,
			s, v...,
		)
	}

	preHandlerTotal := 0
	handlerTotal := 0
	preHandlerFiles := make([]string, 0)
	handlerFilesMap := make(map[string]struct{})
	handlerFiles := make([]string, 0)

	prog.ProcessInfof("parse project in fs: %v, path: %v", filesystem, c.info)
	prog.ProcessInfof("calculate total size of project")
	start := time.Now()
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
				handlerFiles = append(handlerFiles, path)
				handlerFilesMap[path] = struct{}{}
			}
			if c.checkLanguagePreHandler(path) == nil {
				preHandlerTotal++
				preHandlerFiles = append(preHandlerFiles, path)
			}
			return nil
		}),
	)
	calculateTime = time.Since(start)
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
	start = time.Now()

	fileContets := make([]*ssareducer.FileContent, 0, preHandlerTotal)
	for fileContent := range c.getFileHandler(
		filesystem, preHandlerFiles, handlerFilesMap,
	) {
		fileContets = append(fileContets, fileContent)

		preHandlerProcess() // notify the process
		// handler
		if language := c.LanguageBuilder; language != nil {
			language.InitHandler(builder)
			language.PreHandlerProject(filesystem, fileContent.AST, builder, fileContent.Path)
		}
	}
	// },
	preHandlerTime = time.Since(start)
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
	start = time.Now()

	// ssareducer.FilesHandler(
	// 	c.ctx, filesystem, handlerFiles,
	// 	func(path string, content []byte) {
	for _, fileContent := range fileContets {
		if _, needBuild := handlerFilesMap[fileContent.Path]; !needBuild {
			continue // skip if not in handlerFilesMap
		}
		path := fileContent.Path
		content := fileContent.Content
		ast := fileContent.AST
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("parse [%s] error %v  ", path, r)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()

		handlerProcess()

		// build
		if err := prog.Build(ast, path, memedit.NewMemEditorByBytes(content), builder); err != nil {
			log.Errorf("parse %#v failed: %v", path, err)
			continue
		}
	}

	parseTime = time.Since(start)
	// if err != nil {
	// 	return nil, utils.Wrap(err, "parse project error")
	// }
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

	var index int
	prevProcess := 0.9
	lock := sync.Mutex{}
	prog.Cache.SaveToDatabase(func(size int) {
		lock.Lock()
		defer lock.Unlock()
		index += size
		process = 0.9 + (float64(index)/float64(total))*0.1
		if (process - prevProcess) > 0.01 { // is 91.0%/92.0%/....
			prog.ProcessInfof("Saving instructions: %d complete(total %d)", index, total)
		}
	})
	saveTime = time.Since(start)
	_ = prevProcess
	return NewProgram(prog, c), nil
}
