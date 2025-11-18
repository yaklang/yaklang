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
	// scan project files
	scanResult, err := ScanProjectFiles(ScanConfig{
		ProgramName:     programName,
		ProgramPath:     programPath,
		FileSystem:      filesystem,
		ExcludeFunc:     c.excludeFile,
		CheckLanguage:   c.checkLanguage,
		CheckPreHandler: c.checkLanguagePreHandler,
		Context:         c.ctx,
	})
	if err != nil {
		return nil, err
	}

	folder2Save = append(folder2Save, scanResult.Folders...)
	handlerTotal = scanResult.HandlerTotal
	handlerFiles = scanResult.HandlerFiles
	preHandlerTotal = scanResult.PreHandlerTotal
	preHandlerFiles = scanResult.PreHandlerFiles
	handlerFilesMap = scanResult.HandlerFilesMap
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
	enableFilePerfLog := c.Config != nil && c.Config.GetCompileFilePerformanceLog()
	// 创建文件性能 recorder
	if enableFilePerfLog && c.filePerformanceRecorder == nil {
		c.filePerformanceRecorder = diagnostics.NewRecorder()
	}
	filePerfRecorder := c.filePerformanceRecorder
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
			fileASTStart := time.Now()
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

			if fileContent.Err != nil {
				prog.ProcessInfof("file %s parse ast error: %v", fileContent.Path, fileContent.Err)
				AstErr = utils.JoinErrors(AstErr,
					utils.Errorf("pre-handler parse file %s error: %v", fileContent.Path, fileContent.Err),
				)
			}

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
			// 记录文件级别的 AST 解析时间
			if enableFilePerfLog {
				fileASTTime := time.Since(fileASTStart)
				// 收集到 recorder（记录所有文件，不设阈值）
				if filePerfRecorder != nil {
					filePerfRecorder.RecordDuration(fmt.Sprintf("AST[%s]", fileContent.Path), fileASTTime)
					if fileASTTime > 100*time.Millisecond { // 只记录超过 100ms 的文件到日志
						log.Infof("[File Performance] AST parse: %s, time: %v", fileContent.Path, fileASTTime)
					}
				}
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
			// Check if this file needs to be compiled (is in handlerFiles)
			// If not, it's an extra file (like XML) that should be kept in filesystem but not compiled
			needsCompile := false
			for _, hf := range handlerFiles {
				if hf == fileContent.Path {
					needsCompile = true
					break
				}
			}
			// If file doesn't need compilation (extra file), ensure it's added to FileList
			// and skip the build step but keep it in filesystem
			if !needsCompile {
				// Ensure extra files (like XML) are added to FileList even if they don't need compilation
				if fileContent.Editor != nil {
					prog.PushEditor(fileContent.Editor)
					prog.PopEditor(true)
				}
				continue
			}
			// Only skip if AST is nil AND the file needs to be compiled
			// Extra files (like XML) may have nil AST but should still be kept in filesystem
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
				fileBuildStart := time.Now()
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("parse [%s] error %v  ", path, r)
						utils.PrintCurrentGoroutineRuntimeStack()
					}
					// 记录文件级别的 Build 时间
					if enableFilePerfLog {
						fileBuildTime := time.Since(fileBuildStart)
						// 收集到 recorder（记录所有文件，不设阈值）
						if filePerfRecorder != nil {
							filePerfRecorder.RecordDuration(fmt.Sprintf("Build[%s]", path), fileBuildTime)
							if fileBuildTime > 100*time.Millisecond { // 只记录超过 100ms 的文件到日志
								log.Infof("[File Performance] Build: %s, time: %v", path, fileBuildTime)
							}
						}
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

	// 输出文件性能汇总表格
	if enableFilePerfLog && filePerfRecorder != nil {
		snapshots := filePerfRecorder.Snapshot()
		if len(snapshots) > 0 {
			table := diagnostics.FormatPerformanceTable("File Compilation Performance Summary", snapshots)
			fmt.Println(table)
		} else {
			fmt.Println("File Performance: no data recorded")
		}
	}

	return NewProgram(prog, c), nil
}
