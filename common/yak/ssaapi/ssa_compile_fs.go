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

func recordFilePerformance(
	recorder *diagnostics.Recorder,
	metricName string,
	logLabel string,
	path string,
	duration time.Duration,
) {
	if recorder == nil {
		return
	}

	recorder.RecordDuration(fmt.Sprintf("%s[%s]", metricName, path), duration)
	if duration > 100*time.Millisecond {
		log.Infof("[File Performance] %s: %s, time: %v", logLabel, path, duration)
	}
}

func buildFileContent(
	prog *ssa.Program,
	builder *ssa.FunctionBuilder,
	fileContent *ssareducer.FileContent,
	ast ssa.FrontAST,
	enableFilePerfLog bool,
	filePerfRecorder *diagnostics.Recorder,
) {
	path := fileContent.Path
	fileBuildStart := time.Now()
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("parse [%s] error %v  ", path, r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
		if enableFilePerfLog {
			recordFilePerformance(filePerfRecorder, "Build", "Build", path, time.Since(fileBuildStart))
		}
	}()

	if err := prog.Build(ast, fileContent.Editor, builder); err != nil {
		log.Errorf("parse %#v failed: %v", path, err)
	} else {
		// Drop duplicate *MemEditor ref from the slice; IR still holds editors via memedit.Range where needed.
		fileContent.Editor = nil
	}
}

func collectCompileTargets(
	prog *ssa.Program,
	fileContents []*ssareducer.FileContent,
	handlerFiles []string,
) []*ssareducer.FileContent {
	handlerFileSet := make(map[string]struct{}, len(handlerFiles))
	for _, path := range handlerFiles {
		handlerFileSet[path] = struct{}{}
	}

	targets := make([]*ssareducer.FileContent, 0, len(handlerFiles))
	for _, fileContent := range fileContents {
		if fileContent == nil {
			continue
		}
		if fileContent.Status == ssareducer.FileStatusFsError {
			log.Errorf("skip file: %s with fs error: %v", fileContent.Path, fileContent.Err)
			prog.ProcessInfof("skip  file: %s with fs error: %v", fileContent.Path, fileContent.Err)
			continue
		}

		if _, needsCompile := handlerFileSet[fileContent.Path]; !needsCompile {
			if fileContent.Editor != nil {
				prog.PushEditor(fileContent.Editor)
				prog.PopEditor(true)
			}
			continue
		}

		if fileContent.Editor == nil {
			log.Errorf("skip file: %s due to nil editor", fileContent.Path)
			prog.ProcessInfof("skip  file: %s due to nil editor", fileContent.Path)
			continue
		}
		if prog.ShouldVisit(fileContent.Editor.GetUrl()) {
			log.Debugf("parse file %s done skip in main build", fileContent.Path)
			continue
		}

		targets = append(targets, fileContent)
	}
	return targets
}

// parseProjectWithFS compiles a whole project from a filesystem.
//
// Pipeline: parallel read/ParseAST is inside f1 only (FilesHandler -> channel). One goroutine
// consumes the channel (PreHandlerProject) and fills fileContents. f3 walks targets and Build
// sequentially — it does not consume the AST channel. Observability: log=info prints
// [ssa.compile.summary]; log=debug prints ssa.compile.phase enter f1_pre_handler / f3_main_build / …
// and ProcessInfof lines are prefixed with the current phase tag.
func (c *Config) parseProjectWithFS(
	filesystem filesys_interface.FileSystem,
	processCallback func(float64, string, ...any),
) (*Program, error) {
	var calculateTime, preHandlerTime, parseTime, finishTime, saveTime time.Duration
	overallStart := time.Now()
	defer func() {
		log.Debugf("calculate time: %v", calculateTime)
		log.Debugf("pre-handler time: %v", preHandlerTime)
		log.Debugf("parse time (main build f3): %v", parseTime)
		log.Debugf("finish time (f4 Finish+metadata): %v", finishTime)
		log.Debugf("save time: %v", saveTime)
		log.Debugf("ssa.compile.phase_segments: %v", calculateTime+preHandlerTime+parseTime+finishTime+saveTime)
		log.Debugf("ssa.compile.wall: %v", time.Since(overallStart))
	}()

	defer func() {
		if r := recover(); r != nil {
			//err = utils.Errorf("parse [%s] error %v  ", path, r)
			log.Errorf("parse project error: %s", r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	wg := sync.WaitGroup{}

	// compilePhase labels UI / callback messages and debug logs so operators can align htop with f1/f3/f5.
	compilePhase := "f0_scan"

	programName := c.GetProgramName()
	programPath := c.programPath
	preHandlerTotal := 0
	handlerTotal := 0
	preHandlerFiles := make([]string, 0)
	handlerFilesMap := make(map[string]struct{})
	handlerFiles := make([]string, 0)

	var err error
	start := time.Now()

	log.Debugf("ssa.compile.phase enter %s", compilePhase)
	processCallback(0.0, fmt.Sprintf("[%s] parse project in fs: %v, path: %v", compilePhase, filesystem, c.GetCodeSource().ToJSONString()))
	processCallback(0.0, fmt.Sprintf("[%s] calculate total size of project", compilePhase))

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
	// Feed the total compile-input bytes into the adaptive IR cache policy.
	// This is runtime tuning input, not persistent project metadata.
	c.Config.SetCompileProjectBytes(scanResult.HandlerBytes)

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
		msg := s
		if len(v) > 0 {
			msg = fmt.Sprintf(s, v...)
		}
		if compilePhase != "" {
			msg = fmt.Sprintf("[%s] %s", compilePhase, msg)
		}
		processCallback(process, msg)
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
	astParseErrLogged := 0
	astParseErrSuppressed := false
	const maxAstParseErrLogs = 20
	fileContents := make([]*ssareducer.FileContent, 0, preHandlerTotal)
	enableFilePerfLog := c.Config != nil && c.Config.GetCompileFilePerformanceLog()
	// 创建文件性能 recorder
	if enableFilePerfLog && c.filePerformanceRecorder == nil {
		c.filePerformanceRecorder = diagnostics.NewRecorder()
	}
	filePerfRecorder := c.filePerformanceRecorder
	// pre handler  0-40%
	f1 := func() error {
		if prog.Cache != nil {
			prog.Cache.DisableInstructionSpill()
		}
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
		for fileContent := range ch {
			fileASTStart := time.Now()
			if fileContent.Status == ssareducer.FileStatusFsError {
				log.Errorf("skip file: %s with fs error: %v", fileContent.Path, fileContent.Err)
				prog.ProcessInfof("skip  file: %s with fs error: %v", fileContent.Path, fileContent.Err)
				continue
			}

			if fileContent.Status == ssareducer.FileParseASTError {
				if astParseErrLogged < maxAstParseErrLogs {
					log.Warnf("parse Ast file: %s error: %s", fileContent.Path, fileContent.Err)
					astParseErrLogged++
				} else if !astParseErrSuppressed {
					astParseErrSuppressed = true
					log.Warnf("too many AST parse errors; suppressing further per-file logs (limit=%d)", maxAstParseErrLogs)
				}
				AstErr = utils.Errorf("parse Ast file: %s error: %s", fileContent.Path, fileContent.Err)
				// continue
			}

			editor := prog.CreateEditor(fileContent.Content, fileContent.Path)
			// editor := prog.CreateEditor([]byte{}, fileContent.Path)

			fileContent.Editor = editor
			fileContent.Content = nil
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
			if enableFilePerfLog {
				recordFilePerformance(filePerfRecorder, "AST", "AST parse", fileContent.Path, time.Since(fileASTStart))
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
			// Release ANTLR caches early: main build doesn't require the pre-handler
			// parsing caches, and keeping them can cause huge heap + GC overhead.
			language.Clearup()
		}
		prog.ProcessInfof("pre-handler parse project finish")
		return nil
	}

	f3 := func() error {
		if prog.Cache != nil {
			prog.Cache.EnableInstructionSpill()
		}
		process = 0.4 // 40%
		// parse project 40%-90%
		prog.ProcessInfof("parse project start")
		handlerNum := 0
		totalToBuild := len(handlerFiles)
		if totalToBuild <= 0 {
			totalToBuild = len(handlerFilesMap)
		}
		handlerProcess := func() {
			handlerNum++
			// Reserve [0.88, 0.90) for program metadata (f4) and [0.90, 1.0] for IR flush (f5).
			process = 0.4 + (float64(handlerNum)/float64(totalToBuild))*0.48
			if process > 0.88 {
				process = 0.88
			}
		}
		prog.SetPreHandler(false)
		start = time.Now()

		compileTargets := collectCompileTargets(prog, fileContents, handlerFiles)

		for _, fileContent := range compileTargets {
			handlerProcess()
			if fileContent.Status == ssareducer.FileParseASTError || fileContent.AST == nil {
				log.Errorf("skip file: %s due to AST parse error or nil AST: %v", fileContent.Path, fileContent.Err)
				prog.ProcessInfof("skip  file: %s due to AST parse error or nil AST: %v", fileContent.Path, fileContent.Err)
				continue
			}
			ast := fileContent.AST
			fileContent.AST = nil // clear AST
			buildFileContent(prog, builder, fileContent, ast, enableFilePerfLog, filePerfRecorder)
		}

		fileContents = make([]*ssareducer.FileContent, 0)
		parseTime = time.Since(start)
		if c.isStop() {
			return ErrContextCancel
		}
		return nil
	}

	f4 := func() error {
		f4Start := time.Now()
		defer func() { finishTime = time.Since(f4Start) }()
		process = 0.88
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
			prog.ProcessInfof("[SSA/persist] program %s saving program metadata (ir_program)", prog.Name)
			metaStart := time.Now()
			prog.UpdateToDatabaseWithWG(&wg)
			since := time.Since(metaStart)
			log.Infof("program %s save to database cost: %s", prog.Name, since)
			prog.ProcessInfof("[SSA/persist] program %s program metadata saved, cost %v", prog.Name, since)
		}
		process = 0.90
		return nil
	}

	f5 := func() error {
		saveStart := time.Now()
		remaining := prog.Cache.CountInstruction()
		persisted := prog.Cache.InstructionPersistedCount()
		total := remaining + persisted
		process = 0.90
		if prog.DatabaseKind != ssa.ProgramCacheMemory {
			prog.ProcessInfof("[SSA/persist] program %s flushing IR cache (remaining=%d persisted=%d total=%d) to database",
				prog.Name, remaining, persisted, total)
		} else {
			prog.ProcessInfof("[SSA/persist] program %s finishing cache instruction(len:%d) (memory only, not saved)", prog.Name, remaining)
		}

		if err := prog.Cache.SaveToDatabase(irSaveProgressCallback(prog, total, persisted, 0.90, 1.0, func(p float64) {
			process = p
		})); err != nil {
			return utils.Errorf("persist IR to database failed: %w", err)
		}
		saveTime = time.Since(saveStart)
		if prog.DatabaseKind != ssa.ProgramCacheMemory {
			prog.ProcessInfof("[SSA/persist] program %s IR cache flush finished, cost %v", prog.Name, saveTime)
		}
		return nil
	}
	f6 := func() error {
		wg.Wait()
		return nil
	}
	wrapPhase := func(phase string, fn func() error) func() error {
		return func() error {
			compilePhase = phase
			log.Debugf("ssa.compile.phase enter %s", compilePhase)
			return fn()
		}
	}
	phaseSteps := []func() error{
		wrapPhase("f1_pre_handler", f1),
		wrapPhase("f2_after_pre", f2),
		wrapPhase("f3_main_build", f3),
		wrapPhase("f4_finish", f4),
		wrapPhase("f5_save_db", f5),
		wrapPhase("f6_wait", f6),
	}
	if rec := c.DiagnosticsRecorder(); rec != nil {
		err = rec.Track("ParseProjectWithFS", phaseSteps...)
	} else {
		for _, step := range phaseSteps {
			if err = step(); err != nil {
				break
			}
		}
	}
	if err != nil {
		return nil, err
	}

	// wall := time.Since(overallStart)
	// totalCompile := calculateTime + preHandlerTime + parseTime + finishTime + saveTime
	// log.Infof(
	// 	"[ssa.compile.summary] program=%s handler_files=%d wall=%s scan=%s pre_handler=%s main_build=%s finish=%s save_instructions=%s phase_sum=%s",
	// 	prog.Name,
	// 	len(handlerFilesMap),
	// 	wall,
	// 	calculateTime,
	// 	preHandlerTime,
	// 	parseTime,
	// 	finishTime,
	// 	saveTime,
	// 	totalCompile,
	// )

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

const irSaveHeartbeatInterval = 5 * time.Second

// irSaveProgressCallback builds a SaveToDatabase progress func: updates optional
// compile bar in [processMin, processMax], logs delta steps (>0.0001 on that
// range), and emits a heartbeat every irSaveHeartbeatInterval while work advances.
func irSaveProgressCallback(prog *ssa.Program, total int, baseSaved int, processMin, processMax float64, setProcess func(float64)) func(int) {
	var mu sync.Mutex
	var index int
	prevP := processMin
	if total > 0 && baseSaved > 0 {
		prevP = processMin + (float64(baseSaved)/float64(total))*(processMax-processMin)
	}
	lastHB := time.Now()
	lastIdxAtHB := 0
	return func(size int) {
		mu.Lock()
		defer mu.Unlock()
		index += size
		effective := baseSaved + index
		var p float64
		if total > 0 {
			p = processMin + (float64(effective)/float64(total))*(processMax-processMin)
		} else {
			p = processMax
		}
		if setProcess != nil {
			setProcess(p)
		}
		if total > 0 && (p-prevP) > 0.0001 {
			prog.ProcessInfof("[SSA/persist] Saving instructions: %d / %d", effective, total)
			prevP = p
		}
		now := time.Now()
		if total > 0 && index > lastIdxAtHB && now.Sub(lastHB) >= irSaveHeartbeatInterval {
			elapsed := now.Sub(lastHB).Seconds()
			if elapsed <= 0 {
				elapsed = 1e-9
			}
			rate := float64(index-lastIdxAtHB) / elapsed
			prog.ProcessInfof("[SSA/persist] IR save heartbeat: %d / %d (~%.0f inst/s over %.0fs)", effective, total, rate, elapsed)
			lastHB = now
			lastIdxAtHB = index
		}
	}
}
