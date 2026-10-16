package ssaapi

import (
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

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
	concurrentUnits       bool
	concurrency           int
	astErrMu              sync.Mutex
	astErr                error
	astParseErrLogged     int
	astParseErrSuppressed bool
	maxAstParseErrLogs    int
}

func (env *prehandleBatchEnv) consume(prefetchedFiles <-chan *ssareducer.FileContent) bool {
	if language := env.c.LanguageBuilder; language != nil && env.builder != nil {
		language.InitHandler(env.builder)
	}
	if env.concurrentUnits && env.concurrency > 1 {
		return env.consumeParallel(prefetchedFiles)
	}
	canceled := false
	for fileContent := range prefetchedFiles {
		if env.c.isStop() {
			canceled = true
			if fileContent != nil {
				fileContent.Release()
			}
			continue
		}
		env.prehandleFile(fileContent)
	}
	return canceled
}

func (env *prehandleBatchEnv) consumeParallel(prefetchedFiles <-chan *ssareducer.FileContent) bool {
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
	h := fnv.New32a()
	ownerKey := owner.Name
	if ownerKey == "" {
		ownerKey = key
	}
	_, _ = h.Write([]byte(ownerKey))
	return int(h.Sum32()%uint32(workers-1)) + 1
}

func (env *prehandleBatchEnv) prehandleFile(fileContent *ssareducer.FileContent) {
	if fileContent == nil {
		return
	}
	defer fileContent.Release()
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
	env.preHandlerProcess()
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
			rootEditor := fileContent.Editor
			owner.RegisterDeferredBuild(ssa.DeferredBuildKindHelper, "extra-file:"+rootEditor.GetUrl(), func() {
				owner.PushEditor(rootEditor)
				owner.PopEditor(true)
			})
		}
	}
	fileContent.AST = nil
}
