package ssaapi

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

// slowPreHandlerFileThreshold is when one file's pre-handler pass is worth
// reporting. Normal files finish in milliseconds; the ones that stall a batch
// run for minutes.
const slowPreHandlerFileThreshold = time.Second

type prehandleBatchEnv struct {
	c                     *Config
	prog                  *ssa.Program
	builder               *ssa.FunctionBuilder
	filesystem            filesys_interface.FileSystem
	fileUnits             map[string]string
	preHandlerFileSet     map[string]struct{}
	handlerFileSet        map[string]struct{}
	preHandlerBuildsFiles bool
	preHandlerProcess     func()
	concurrency           int
	astErrMu              sync.Mutex
	astErr                error
	astParseErrLogged     int
	astParseErrSuppressed bool
	maxAstParseErrLogs    int
	// unitAssign sticks one library to one worker. workerLoad counts files
	// so the next library lands on the least busy worker instead of a hash
	// bucket. Worker 0 stays reserved for units that share the application.
	unitAssign map[string]int
	workerLoad []int
}

// unitWorkerCounts is the only serial-versus-parallel decision for one batch.
// Prehandle and body build each get a goroutine count; 1 is the serial run of
// the same code. drainUnscheduled is for frontends that do not schedule their
// own upstream libraries.
func unitWorkerCounts(c *Config, prog *ssa.Program) (prehandleWorkers, buildWorkers int, drainUnscheduled bool) {
	configured := 1
	if c != nil {
		configured = c.GetCompileConcurrency()
	}
	if configured < 1 {
		configured = 1
	}
	prehandleWorkers = 1
	if c != nil {
		if pre, ok := c.LanguageBuilder.(ssa.ConcurrentUnitPreHandler); ok {
			if pre.SupportsConcurrentUnitPreHandler() {
				prehandleWorkers = configured
			}
		} else if prog != nil && prog.ConcurrentUnitBuildEnabled() {
			prehandleWorkers = configured
		}
	}
	buildWorkers = 1
	drainUnscheduled = true
	if c != nil {
		if build, ok := c.LanguageBuilder.(ssa.ConcurrentUnitBuilder); ok && build.SupportsConcurrentUnitBuild() {
			buildWorkers = configured
			drainUnscheduled = false
		}
	}
	return prehandleWorkers, buildWorkers, drainUnscheduled
}

func (env *prehandleBatchEnv) consume(prefetchedFiles <-chan *ssareducer.FileContent) bool {
	if language := env.c.LanguageBuilder; language != nil && env.builder != nil {
		language.InitHandler(env.builder)
	}
	workers := env.concurrency
	if workers < 1 {
		workers = 1
	}
	queues := make([]chan *ssareducer.FileContent, workers)
	var wg sync.WaitGroup
	var canceled atomic.Bool
	for i := 0; i < workers; i++ {
		queues[i] = make(chan *ssareducer.FileContent, 8)
		wg.Add(1)
		go func(ch <-chan *ssareducer.FileContent) {
			defer wg.Done()
			for fileContent := range ch {
				if env.c.isStop() {
					canceled.Store(true)
					if fileContent != nil {
						fileContent.Release()
					}
					continue
				}
				env.prehandleFile(fileContent)
			}
		}(queues[i])
	}
	for fileContent := range prefetchedFiles {
		if env.c.isStop() {
			canceled.Store(true)
			if fileContent != nil {
				fileContent.Release()
			}
			continue
		}
		key := ""
		if fileContent != nil {
			key = env.fileUnits[fileContent.Path]
		}
		queues[env.unitWorker(key, workers)] <- fileContent
	}
	for _, ch := range queues {
		close(ch)
	}
	wg.Wait()
	return canceled.Load()
}

func (env *prehandleBatchEnv) unitWorker(key string, workers int) int {
	if workers <= 1 {
		return 0
	}
	owner := env.prog.EnsureLibraryForCompileUnit(key)
	if owner == nil || owner == env.prog.GetApplication() {
		return 0
	}
	if workers == 2 {
		return 1
	}
	ownerKey := owner.Name
	if ownerKey == "" {
		ownerKey = key
	}
	if env.unitAssign == nil {
		env.unitAssign = make(map[string]int)
		env.workerLoad = make([]int, workers)
	}
	if worker, ok := env.unitAssign[ownerKey]; ok {
		env.workerLoad[worker]++
		return worker
	}
	best := 1
	for i := 2; i < workers; i++ {
		if env.workerLoad[i] < env.workerLoad[best] {
			best = i
		}
	}
	env.unitAssign[ownerKey] = best
	env.workerLoad[best]++
	return best
}

func (env *prehandleBatchEnv) prehandleFile(fileContent *ssareducer.FileContent) {
	if fileContent == nil {
		return
	}
	defer fileContent.Release()
	// A file that takes minutes in here pins one worker and stalls its whole
	// batch, and the batch log only prints after the batch ends — so the run
	// looks hung with no clue which file did it. Report the culprit by name.
	filePath := fileContent.Path
	fileStarted := time.Now()
	defer func() {
		if elapsed := time.Since(fileStarted); elapsed > slowPreHandlerFileThreshold {
			log.Warnf("[ssa-compile] slow pre-handler file %s took %v", filePath, elapsed)
		}
	}()
	// Progress counts finished files: the heavy work (pre-handler + inline file build) is
	// below, and a file picked up by a worker must not advance the bar before it is done.
	if env.preHandlerProcess != nil {
		defer env.preHandlerProcess()
	}
	unitKey := env.fileUnits[fileContent.Path]
	owner := env.prog.EnsureLibraryForCompileUnit(unitKey)
	if owner == nil {
		owner = env.prog
	}
	owner.BeginCompileUnit(unitKey)
	defer owner.EndCompileUnit()
	// Hold this library's directory slot for the skeleton. An import on
	// another worker must not compile it: that enters this builder
	// (editor, build support, range and class registration). The slot is
	// released with the file, before deferred import linking.
	if env.prog.ConcurrentUnitBuildEnabled() && owner != env.prog && owner.Name != "" {
		app := owner.GetApplication()
		if app == nil {
			app = owner
		}
		releasePackage := app.OccupyDirectoryPackage(owner.Name)
		defer releasePackage()
	}

	if fileContent.Status == ssareducer.FileStatusFsError {
		env.prog.RecordCompileDiagnostic("file", fileContent.Path, fmt.Sprint(fileContent.Err))
		log.Errorf("skip file: %s with fs error: %v", fileContent.Path, fileContent.Err)
		env.prog.ProcessInfof("skip  file: %s with fs error: %v", fileContent.Path, fileContent.Err)
		return
	}
	if fileContent.Status == ssareducer.FileParseASTError {
		env.prog.RecordCompileDiagnostic("ast", fileContent.Path, fmt.Sprint(fileContent.Err))
		env.astErrMu.Lock()
		if env.astParseErrLogged < env.maxAstParseErrLogs {
			log.Warnf("parse Ast file: %s error: %s", fileContent.Path, fileContent.Err)
			env.astParseErrLogged++
		} else if !env.astParseErrSuppressed {
			env.astParseErrSuppressed = true
			log.Warnf("too many AST parse errors; suppressing further per-file logs (limit=%d)", env.maxAstParseErrLogs)
		}
		env.astErr = utils.Errorf("parse Ast file: %s error: %s", fileContent.Path, fileContent.Err)
		env.astErrMu.Unlock()
	}
	editor := env.prog.CreateEditor(fileContent.Content, fileContent.Path)
	fileContent.Editor = editor
	fileContent.Content = nil
	if fileContent.Err != nil {
		env.prog.ProcessInfof("file %s parse ast error: %v", fileContent.Path, fileContent.Err)
		env.astErrMu.Lock()
		env.astErr = utils.JoinErrors(env.astErr, utils.Errorf("pre-handler parse file %s error: %v", fileContent.Path, fileContent.Err))
		env.astErrMu.Unlock()
	}
	unitBuilder := env.prog.GetOrCreateUnitBuilder(unitKey, env.builder)
	if unitBuilder == nil {
		unitBuilder = env.builder
	}
	if editor != nil && unitBuilder != nil {
		unitBuilder.SetEditor(editor)
	}
	if language := env.c.LanguageBuilder; language != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					env.prog.RecordCompileDiagnostic("prehandler", fileContent.Path, fmt.Sprint(r))
					log.Errorf("pre-handler parse [%s] error %v  ", fileContent.Path, r)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			err := language.PreHandlerProject(env.filesystem, fileContent.AST, unitBuilder, editor)
			if err != nil {
				var configError *ssa.ProjectConfigError
				if errors.As(err, &configError) {
					log.Warnf("%v", err)
				} else {
					env.prog.RecordCompileDiagnostic("prehandler", fileContent.Path, err.Error())
					log.Errorf("pre-handler parse [%s] error %v", fileContent.Path, err)
				}
			}
		}()
	}
	if env.preHandlerBuildsFiles {
		ssa.ReleaseASTRoot(fileContent.AST)
	}
	if _, needBuild := env.preHandlerFileSet[fileContent.Path]; needBuild {
		_, needsCompile := env.handlerFileSet[fileContent.Path]
		switch {
		case needsCompile && fileContent.AST != nil && !env.preHandlerBuildsFiles:
			ast := fileContent.AST
			path := fileContent.Path
			env.prog.RegisterFileBuild(path, editor, unitBuilder, func(fileBuilder *ssa.FunctionBuilder) {
				fileBuildStart := time.Now()
				defer func() {
					if d := time.Since(fileBuildStart); d > 100*time.Millisecond {
						log.Debugf("[file build] %s cost %v", path, d)
					}
				}()
				if err := env.c.LanguageBuilder.BuildFromAST(ast, fileBuilder); err != nil {
					log.Errorf("file build [%s] failed: %v", path, err)
				}
			})
		case !needsCompile && fileContent.Editor != nil:
			if env.c.hasYakEntryFiles() {
				// CreateEditor already retained this source. An unexecuted
				// include target must not enter the compiled FileList.
				break
			}
			rootEditor := fileContent.Editor
			owner.RegisterDeferredBuild(ssa.DeferredBuildKindHelper, "extra-file:"+rootEditor.GetUrl(), func() {
				owner.PushEditor(rootEditor)
				owner.PopEditor(true)
			})
		}
	}
	fileContent.AST = nil
}
