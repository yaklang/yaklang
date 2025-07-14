package ssaapi

import (
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/databasex"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

type SaveFolder struct {
	name string
	path []string
}

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

	wg := sync.WaitGroup{}

	programPath := c.programPath
	preHandlerTotal := 0
	handlerTotal := 0
	preHandlerFiles := make([]string, 0)
	handlerFilesMap := make(map[string]struct{})
	handlerFiles := make([]string, 0)

	var err error
	start := time.Now()

	processCallback(0.0, fmt.Sprintf("parse project in fs: %v, path: %v", filesystem, c.info))
	processCallback(0.0, "calculate total size of project")

	db := ssadb.GetDB()
	folderSave := databasex.NewSave[SaveFolder](func(t []SaveFolder) {
		utils.GormTransaction(db, func(tx *gorm.DB) error {
			for _, sf := range t {
				ssadb.SaveFolder(tx, sf.name, sf.path)
			}
			return nil
		})
	})
	if c.enableDatabase != ssa.ProgramCacheMemory {
		folderSave.Save(SaveFolder{
			name: c.ProgramName,
			path: []string{"/"},
		})
	}

	// get total size
	err = filesys.Recursive(programPath,
		filesys.WithFileSystem(filesystem),
		filesys.WithContext(c.ctx),
		filesys.WithDirStat(func(s string, fi fs.FileInfo) error {
			folder, name := filesystem.PathSplit(s)
			if name == "test" || name == ".git" {
				return filesys.SkipDir
			}
			folders := []string{c.ProgramName}
			folders = append(folders,
				strings.Split(folder, string(c.fs.GetSeparators()))...,
			)
			if c.enableDatabase != ssa.ProgramCacheMemory {
				folderSave.Save(SaveFolder{
					name: name,
					path: folders,
				})
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

	prog, builder, err := c.init(filesystem, handlerTotal)
	if err != nil {
		return nil, err
	}

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

	var AstErr error
	fileContents := make([]*ssareducer.FileContent, 0, preHandlerTotal)
	// pre handler  0-40%
	f1 := func() error {
		preHandlerNum := 0
		preHandlerProcess := func() {
			preHandlerNum++
			process = 0 + (float64(preHandlerNum)/float64(preHandlerTotal))*0.4
		}
		prog.SetPreHandler(true)
		prog.ProcessInfof("pre-handler parse project in fs: %v, path: %v", filesystem, c.info)
		start = time.Now()
		for fileContent := range c.getFileHandler(
			filesystem, preHandlerFiles, handlerFilesMap,
		) {
			fileContents = append(fileContents, fileContent)
			if fileContent.Err != nil {
				AstErr = utils.JoinErrors(AstErr,
					utils.Errorf("pre-handler parse file %s error: %v", fileContent.Path, fileContent.Err),
				)
			}

			preHandlerProcess() // notify the process
			// handler
			if language := c.LanguageBuilder; language != nil {
				language.InitHandler(builder)
				language.PreHandlerProject(filesystem, fileContent.AST, builder, fileContent.Path)
			}
		}
		preHandlerTime = time.Since(start)
		if AstErr != nil && c.strictMode {
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
			process = 0.4 + (float64(handlerNum)/float64(handlerTotal))*0.5
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
		if c.isStop() {
			return ErrContextCancel
		}
		return nil
	}

	f4 := func() error {
		process = 0.9 // %90
		prog.Finish()
		if prog.DatabaseKind != ssa.ProgramCacheMemory { // save program
			log.Errorf("program %s save to database", prog.Name)
			start := time.Now()
			prog.UpdateToDatabaseWithWG(&wg)
			since := time.Since(start)
			log.Errorf("program %s save to database cost: %s", prog.Name, since)
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
			if (process - prevProcess) > 0.001 { // is 90.1%/90.2%/....
				prog.ProcessInfof("Saving instructions: %d complete(total %d)", index, total)
				prevProcess = process
			}
		})
		saveTime = time.Since(start)
		return nil
	}
	f6 := func() error {
		wg.Wait()
		folderSave.Close()
		return nil
	}
	ssaprofile.ProfileAddWithError(true, "ParseProjectWithFS", f1, f2, f3, f4, f5, f6)

	return NewProgram(prog, c), nil
}
