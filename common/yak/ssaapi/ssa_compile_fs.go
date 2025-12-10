package ssaapi

import (
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

type SaveFolder struct {
	name string
	path []string
}

func (c *Config) parseProjectWithFS(
	filesystem filesys_interface.FileSystem,
	processCallback func(float64, string, ...any),
) (*Program, error) {

	var calculateTime, preHandlerTime, parseTime, saveTime time.Duration
	defer func() {
		log.Debugf("calculate time: %v", calculateTime)
		log.Debugf("pre-handler time: %v", preHandlerTime)
		log.Debugf("parse time: %v", parseTime)
		log.Debugf("save time: %v", saveTime)
	}()

	defer func() {
		if r := recover(); r != nil {
			//err = utils.Errorf("parse [%s] error %v  ", path, r)
			log.Errorf("parse project error: %s", r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	wg := sync.WaitGroup{}

	programName := c.GetProgramName()
	programPath := c.programPath
	preHandlerTotal := 0
	handlerTotal := 0
	preHandlerFiles := make([]string, 0)
	handlerFilesMap := make(map[string]struct{})
	handlerFiles := make([]string, 0)

	var err error
	start := time.Now()

	processCallback(0.0, fmt.Sprintf("parse project in fs: %v, path: %v", filesystem, c.GetCodeSource().ToJSONString()))
	processCallback(0.0, "calculate total size of project")

	folder2Save := make([][]string, 0)
	if programName != "" {
		folder2Save = append(folder2Save, []string{"/", programName})
	}

	filesystem = c.swapLanguageFs(filesystem)
	// get total size
	err = filesys.Recursive(programPath,
		filesys.WithFileSystem(filesystem),
		filesys.WithContext(c.ctx),
		filesys.WithDirStat(func(fullPath string, fi fs.FileInfo) error {
			// check folder folderName
			_, folderName := filesystem.PathSplit(fullPath)
			if folderName == "test" || folderName == ".git" {
				return filesys.SkipDir
			}

			folders := []string{programName}
			folders = append(folders,
				strings.Split(fullPath, string(c.fs.GetSeparators()))...,
			)
			folder2Save = append(folder2Save, folders)
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
			}
			if c.checkLanguagePreHandler(path) == nil {
				preHandlerTotal++
				preHandlerFiles = append(preHandlerFiles, path)
				handlerFilesMap[path] = struct{}{}
			}
			return nil
		}),
	)
	calculateTime = time.Since(start)
	if err != nil {
		return nil, err
	}

	prog, builder, err := c.init(filesystem, handlerTotal)
	if err != nil {
		return nil, err
	}

	wg.Add(1)
	go func() {
		wg.Done()
		for _, folder := range folder2Save {
			_ = folder
			prog.SaveFolder(folder)
		}
	}()

	process := 0.0
	prog.ProcessInfof = func(s string, v ...any) {
		processCallback(
			process,
			s, v...,
		)
	}

	if c.isStop() {
		return nil, ErrContextCancel
	}
	if (handlerTotal + preHandlerTotal) == 0 {
		return nil, ErrNoFoundCompiledFile
	}
	if preHandlerTotal < handlerTotal {
		preHandlerTotal = handlerTotal
		preHandlerFiles = handlerFiles
	}
	prog.ProcessInfof("calculate total size of project finish preHandler(len:%d) build(len:%d)", preHandlerTotal, handlerTotal)
	defer c.LanguageBuilder.Clearup()

	var AstErr error
	fileContents := make([]*ssareducer.FileContent, 0, preHandlerTotal)
	// pre handler  0-40%
	f1 := func() error {
		preHandlerNum := 0
		preHandlerProcess := func() {
			preHandlerNum++
			process = 0 + (float64(preHandlerNum)/float64(preHandlerTotal))*0.4
			if process > 0.4 {
				process = 0.4
			}
		}
		prog.SetPreHandler(true)
		prog.ProcessInfof("pre-handler parse project in fs: %v, path: %v", filesystem, c.GetCodeSource().ToJSONString())
		start = time.Now()

		ch := c.GetFileHandler(
			filesystem, preHandlerFiles, handlerFilesMap,
		)
		// diagnostics.DumpHeap(diagnostics.WithName("ast"))
		for fileContent := range ch {
			if fileContent.Status == ssareducer.FileStatusFsError {
				log.Errorf("skip file: %s with fs error: %v", fileContent.Path, fileContent.Err)
				prog.ProcessInfof("skip  file: %s with fs error: %v", fileContent.Path, fileContent.Err)
				continue
			}

			if fileContent.Status == ssareducer.FileParseASTError {
				log.Errorf("parse Ast file: %s error: %s", fileContent.Path, fileContent.Err)
				AstErr = utils.Errorf("parse Ast file: %s error: %s", fileContent.Path, fileContent.Err)
				// continue
			}

			editor := prog.CreateEditor(fileContent.Content, fileContent.Path)
			// editor := prog.CreateEditor([]byte{}, fileContent.Path)

			fileContent.Editor = editor
			fileContents = append(fileContents, fileContent)

			preHandlerProcess() // notify the process
			// handler
			if language := c.LanguageBuilder; language != nil {
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Errorf("pre-handler parse [%s] error %v  ", fileContent.Path, r)
							utils.PrintCurrentGoroutineRuntimeStack()
						}
					}()
					language.InitHandler(builder)
					err = language.PreHandlerProject(filesystem, fileContent.AST, builder, editor)
					if err != nil {
						log.Errorf("pre-handler parse [%s] error %v", fileContent.Path, err)
					}
				}()
			}
		}
		preHandlerTime = time.Since(start)
		if AstErr != nil && c.GetCompileStrictMode() {
			return utils.Errorf("pre-handler parse project error: %v", AstErr)
		}
		if c.isStop() {
			return ErrContextCancel
		}
		return nil
	}

	f2 := func() error {
		if language := c.LanguageBuilder; language != nil {
			language.AfterPreHandlerProject(builder)
		}
		prog.ProcessInfof("pre-handler parse project finish")
		return nil
	}

	f3 := func() error {
		process = 0.4 // 40%
		// parse project 40%-90%
		prog.ProcessInfof("parse project start")
		handlerNum := 0
		handlerProcess := func() {
			handlerNum++
			process = 0.4 + (float64(handlerNum)/float64(len(handlerFilesMap)))*0.5
			if process > 0.9 {
				process = 0.9 // limit to 90%
			}
		}
		prog.SetPreHandler(false)
		start = time.Now()

		// ssareducer.FilesHandler(
		// 	c.ctx, filesystem, handlerFiles,
		// 	func(path string, content []byte) {
		for _, fileContent := range fileContents {
			if _, needBuild := handlerFilesMap[fileContent.Path]; !needBuild {
				continue // skip if not in handlerFilesMap
			}
			handlerProcess()
			if fileContent.Status == ssareducer.FileStatusFsError {
				log.Errorf("skip file: %s with fs error: %v", fileContent.Path, fileContent.Err)
				prog.ProcessInfof("skip  file: %s with fs error: %v", fileContent.Path, fileContent.Err)
				continue
			}
			if fileContent.Status == ssareducer.FileParseASTError || fileContent.AST == nil {
				log.Errorf("skip file: %s due to AST parse error or nil AST: %v", fileContent.Path, fileContent.Err)
				prog.ProcessInfof("skip  file: %s due to AST parse error or nil AST: %v", fileContent.Path, fileContent.Err)
				continue
			}
			// log.Infof("visited file: ", prog.GetIncludeFiles())
			if prog.ShouldVisit(fileContent.Editor.GetUrl()) {
				log.Infof("parse file %s done skip in main build", fileContent.Path)
				continue
			}
			path := fileContent.Path
			ast := fileContent.AST
			fileContent.AST = nil // clear AST
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("parse [%s] error %v  ", path, r)
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()
				// build
				if err := prog.Build(ast, fileContent.Editor, builder); err != nil {
					log.Errorf("parse %#v failed: %v", path, err)
				}
			}()
		}
		fileContents = make([]*ssareducer.FileContent, 0)
		parseTime = time.Since(start)
		if c.isStop() {
			return ErrContextCancel
		}
		return nil
	}

	f4 := func() error {
		process = 0.9 // %90
		prog.Finish()
		if prog.DatabaseKind != ssa.ProgramCacheMemory { // save program
			start := time.Now()
			prog.UpdateToDatabaseWithWG(&wg)
			since := time.Since(start)
			log.Infof("program %s save to database cost: %s", prog.Name, since)
		}
		return nil
	}

	f5 := func() error {
		total := prog.Cache.CountInstruction()
		process = 0.9
		prog.ProcessInfof("program %s finishing save cache instruction(len:%d) to database", prog.Name, total) // %90

		var index int
		prevProcess := 0.9
		_ = prevProcess
		lock := sync.Mutex{}
		prog.Cache.SaveToDatabase(func(size int) {
			lock.Lock()
			defer lock.Unlock()
			index += size
			process = 0.9 + (float64(index)/float64(total))*0.1
			if (process - prevProcess) > 0.0001 { // is 90.01%/90.02%/....
				prog.ProcessInfof("Saving instructions: %d complete(total %d)", index, total)
				prevProcess = process
			}
		})
		saveTime = time.Since(start)
		return nil
	}
	f6 := func() error {
		wg.Wait()
		return nil
	}
	if err := c.DiagnosticsTrack("ParseProjectWithFS", f1, f2, f3, f4, f5, f6); err != nil {
		return nil, err
	}

	return NewProgram(prog, c), nil
}
