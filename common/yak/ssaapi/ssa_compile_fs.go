package ssaapi

import (
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
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
	folder2Save := make([][]string, 0)

	var err error
	filePerfRecorder := c.DiagnosticsRecorder()

	calculateTime, err = filePerfRecorder.ForKind(diagnostics.TrackKindGeneral).TrackLow("calculate project size", func() error {
		processCallback(0.0, fmt.Sprintf("parse project in fs: %v, path: %v", filesystem, c.GetCodeSource().ToJSONString()))
		processCallback(0.0, "calculate total size of project")
		if programName != "" {
			folder2Save = append(folder2Save, []string{"/", programName})
		}

		filesystem = c.swapLanguageFs(filesystem)
		scanResult, errScan := ScanProjectFiles(ScanConfig{
			ProgramName:     programName,
			ProgramPath:     programPath,
			FileSystem:      filesystem,
			ExcludeFunc:     c.excludeFile,
			CheckLanguage:   c.checkLanguage,
			CheckPreHandler: c.checkLanguagePreHandler,
			Context:         c.ctx,
		})
		if errScan != nil {
			return errScan
		}

		folder2Save = append(folder2Save, scanResult.Folders...)
		handlerTotal = scanResult.HandlerTotal
		handlerFiles = scanResult.HandlerFiles
		preHandlerTotal = scanResult.PreHandlerTotal
		preHandlerFiles = scanResult.PreHandlerFiles
		handlerFilesMap = scanResult.HandlerFilesMap
		return nil
	})
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
	// 统一使用 filePerfRecorder；RunWithCurrentRecorderErr 管理 currentRecorder 生命周期
	prog.SetDiagnosticsRecorder(filePerfRecorder)
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

		preHandlerTime, _ = filePerfRecorder.ForKind(ssa.TrackKindAST).TrackLow("pre-handler phase", func() error {
			ch := c.GetFileHandler(
				filesystem, preHandlerFiles, handlerFilesMap,
			)
			for fileContent := range ch {
			func() {
				path := fileContent.Path
				astName := fmt.Sprintf("AST[%s]", path)
				work := func() error {
					if fileContent.Status == ssareducer.FileStatusFsError {
						log.Errorf("skip file: %s with fs error: %v", fileContent.Path, fileContent.Err)
						prog.ProcessInfof("skip  file: %s with fs error: %v", fileContent.Path, fileContent.Err)
						return nil
					}
					if fileContent.Status == ssareducer.FileParseASTError {
						log.Errorf("parse Ast file: %s error: %s", fileContent.Path, fileContent.Err)
						AstErr = utils.Errorf("parse Ast file: %s error: %s", fileContent.Path, fileContent.Err)
					}
					editor := prog.CreateEditor(fileContent.Content, fileContent.Path)
					fileContent.Editor = editor
					fileContents = append(fileContents, fileContent)
					if fileContent.Err != nil {
						prog.ProcessInfof("file %s parse ast error: %v", fileContent.Path, fileContent.Err)
						AstErr = utils.JoinErrors(AstErr,
							utils.Errorf("pre-handler parse file %s error: %v", fileContent.Path, fileContent.Err),
						)
					}
					preHandlerProcess()
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
					return nil
				}
				sizeClosure := func() error {
					if len(fileContent.Content) > 0 {
						filePerfRecorder.AddSizeToEntry(astName, int64(len(fileContent.Content)))
					}
					return nil
				}
				_, _ = filePerfRecorder.ForKind(ssa.TrackKindAST).Track(astName, work, sizeClosure)
			}()
		}
		return nil
		})
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

		var parseErr error
		parseErr = func() error {
			// O(1) lookup: handlerFilesSet for needsCompile check
			handlerFilesSet := make(map[string]struct{}, len(handlerFiles))
			for _, hf := range handlerFiles {
				handlerFilesSet[hf] = struct{}{}
			}
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
				// Check if this file needs to be compiled (is in handlerFiles)
				if _, needsCompile := handlerFilesSet[fileContent.Path]; !needsCompile {
					if fileContent.Editor != nil {
						prog.PushEditor(fileContent.Editor)
						prog.PopEditor(true)
					}
					continue
				}
				if fileContent.Status == ssareducer.FileParseASTError || fileContent.AST == nil {
					log.Errorf("skip file: %s due to AST parse error or nil AST: %v", fileContent.Path, fileContent.Err)
					prog.ProcessInfof("skip  file: %s due to AST parse error or nil AST: %v", fileContent.Path, fileContent.Err)
					continue
				}
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
					if e := prog.Build(ast, fileContent.Editor, builder); e != nil {
						log.Errorf("parse %#v failed: %v", path, e)
					}
				}()
			}
			return nil
		}()
		_ = parseErr
		fileContents = make([]*ssareducer.FileContent, 0)
		if c.isStop() {
			return ErrContextCancel
		}
		return nil
	}

	f4 := func() error {
		process = 0.9 // %90
		prog.Finish()
		// 在保存到数据库之前，设置增量编译信息（如果存在）
		if baseProgramName := c.GetBaseProgramName(); baseProgramName != "" {
			prog.BaseProgramName = baseProgramName
		}
		if len(c.fileHashMap) > 0 {
			prog.FileHashMap = c.fileHashMap
		}
		// 如果启用了增量编译，确保 IsOverlay 被设置
		// 即使没有 baseProgramName 和 fileHashMap（第一次增量编译），也设置一个空的 FileHashMap 作为标记
		// 这样 UpdateToDatabaseWithWG 会设置 IsOverlay = true
		if c.GetEnableIncrementalCompile() && prog.FileHashMap == nil {
			prog.FileHashMap = make(map[string]int)
		}
		if prog.DatabaseKind != ssa.ProgramCacheMemory { // save program
			_ = filePerfRecorder.ForKind(ssa.TrackKindDatabase).TrackLowLog(fmt.Sprintf("program %s save to database", prog.Name), func() error {
				prog.UpdateToDatabaseWithWG(&wg)
				return nil
			})
		}
		return nil
	}

	f5 := func() error {
		total := prog.Cache.CountInstruction()
		process = 0.9
		if prog.DatabaseKind != ssa.ProgramCacheMemory {
			prog.ProcessInfof("program %s finishing save cache instruction(len:%d) to database", prog.Name, total)
		} else {
			prog.ProcessInfof("program %s finishing cache instruction(len:%d) (memory only, not saved)", prog.Name, total)
		}

		var index int
		prevProcess := 0.9
		_ = prevProcess
		lock := sync.Mutex{}
		var saveErr error
		saveTime, saveErr = filePerfRecorder.ForKind(ssa.TrackKindDatabase).TrackLow("save cache instructions", func() error {
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
			return nil
		})
		return saveErr
	}
	f6 := func() error {
		wg.Wait()
		return nil
	}
	if err := diagnostics.RunWithCurrentRecorderErr(filePerfRecorder, func() error {
		for _, fn := range []func() error{f1, f2, f3, f4, f5, f6} {
			if err := fn(); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// 展示层：表格由 ParseProject 的 defer LogDiagnostics 统一打印一次，此处仅输出总耗时
	totalCompile := calculateTime + preHandlerTime + saveTime
	diagnostics.LogLow(ssa.TrackKindBuild, "", fmt.Sprintf("total compile elapsed %v", totalCompile))
	diagnostics.LogHeapSnapshot("ssa_compile_project_end", true)

	return NewProgram(prog, c), nil
}
