package ssaapi

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

var (
	ErrContextCancel       error = errors.New("context cancel")
	ErrNoFoundCompiledFile error = errors.New("not found can compiled file")
)

const (
	antlrWorkerStateKey         = "antlr_worker_state"
	largeProjectByteCap         = 16 * 1024 * 1024
	defaultASTMemoryBudgetRatio = 60
	defaultASTMemoryBudgetMax   = int64(16 * 1024 * 1024 * 1024)
	minAutoASTMemoryBudget      = int64(2 * 1024 * 1024 * 1024)
	defaultASTSlotCost          = int64(1024 * 1024 * 1024)
	defaultLargeProjectGC       = 100
)

var (
	antlrCacheResetEveryFilesOnce   sync.Once
	antlrCacheResetEveryFilesCached int
	antlrCacheResetEveryBytesOnce   sync.Once
	antlrCacheResetEveryBytesCached int64
)

func antlrCacheResetEveryFiles() int {
	antlrCacheResetEveryFilesOnce.Do(func() {
		antlrCacheResetEveryFilesCached = 25
		if raw := strings.TrimSpace(os.Getenv("YAK_ANTLR_CACHE_RESET_FILES")); raw != "" {
			if v, err := strconv.Atoi(raw); err == nil {
				antlrCacheResetEveryFilesCached = v
			}
		}
		if antlrCacheResetEveryFilesCached <= 0 {
			antlrCacheResetEveryFilesCached = 0
		}
	})
	return antlrCacheResetEveryFilesCached
}

func antlrCacheResetEveryBytes() int64 {
	antlrCacheResetEveryBytesOnce.Do(func() {
		antlrCacheResetEveryBytesCached = 8 * 1024 * 1024
		if raw := strings.TrimSpace(os.Getenv("YAK_ANTLR_CACHE_RESET_BYTES")); raw != "" {
			switch strings.ToLower(raw) {
			case "0", "false", "no", "off", "disable", "disabled":
				antlrCacheResetEveryBytesCached = 0
			default:
				if v, err := utils.ToBytes(raw); err == nil {
					antlrCacheResetEveryBytesCached = int64(v)
				} else if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
					antlrCacheResetEveryBytesCached = v
				}
			}
		}
		if antlrCacheResetEveryBytesCached <= 0 {
			antlrCacheResetEveryBytesCached = 0
		}
	})
	return antlrCacheResetEveryBytesCached
}

func languagePreHandlerBuildsFiles(language ssaconfig.Language) bool {
	switch language {
	case ssaconfig.C, ssaconfig.GO, ssaconfig.JAVA, ssaconfig.PHP, ssaconfig.JS, ssaconfig.TS, ssaconfig.PYTHON:
		return true
	default:
		return false
	}
}

type astBuildWindowDecision struct {
	window           int
	budgetBytes      int64
	slotCostBytes    int64
	manualOverride   bool
	largeProject     bool
	diagnosticsHeavy bool
	budgetSource     string
}

type largeProjectGCDecision struct {
	percent        int
	manualOverride bool
	largeProject   bool
	source         string
}

func parseMemoryBudgetEnv(raw string) (int64, bool) {
	normalized := strings.TrimSpace(strings.ToLower(raw))
	switch normalized {
	case "", "auto":
		return 0, false
	case "0", "false", "no", "off", "disable", "disabled":
		return 0, true
	}
	if v, err := utils.ToBytes(raw); err == nil {
		return int64(v), true
	}
	if v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil {
		return v, true
	}
	return 0, false
}

func systemMemoryTotalBytes() int64 {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "MemTotal:" {
			continue
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb * 1024
	}
	return 0
}

func autoASTMemoryBudgetBytes() (int64, string) {
	if raw := strings.TrimSpace(os.Getenv("YAK_SSA_AST_MEMORY_BUDGET")); raw != "" {
		if budget, ok := parseMemoryBudgetEnv(raw); ok {
			return budget, "env:YAK_SSA_AST_MEMORY_BUDGET"
		}
		log.Warnf("invalid YAK_SSA_AST_MEMORY_BUDGET=%q, falling back to system memory", raw)
	}

	total := systemMemoryTotalBytes()
	if total <= 0 {
		return 0, "unknown"
	}
	budget := total * defaultASTMemoryBudgetRatio / 100
	if budget > defaultASTMemoryBudgetMax {
		budget = defaultASTMemoryBudgetMax
	}
	if budget < minAutoASTMemoryBudget && total >= minAutoASTMemoryBudget {
		budget = minAutoASTMemoryBudget
	}
	return budget, "auto:system-memory"
}

func astSlotCostBytes(language ssaconfig.Language) int64 {
	cost := defaultASTSlotCost
	switch language {
	case ssaconfig.PHP:
		cost = 4 * defaultASTSlotCost
	case ssaconfig.JAVA, ssaconfig.JS, ssaconfig.TS, ssaconfig.PYTHON:
		cost = 2 * defaultASTSlotCost
	case ssaconfig.C, ssaconfig.GO:
		cost = defaultASTSlotCost
	}
	if cost < defaultASTSlotCost {
		return defaultASTSlotCost
	}
	return cost
}

func clampASTBuildWindow(window, concurrency int) int {
	if window < 1 {
		window = 1
	}
	if concurrency > 0 && window > concurrency {
		window = concurrency
	}
	return window
}

func (c *Config) resolveASTBuildWindow(concurrency int) astBuildWindowDecision {
	decision := astBuildWindowDecision{
		largeProject: c != nil && c.GetCompileProjectBytes() >= largeProjectByteCap,
	}
	if !decision.largeProject {
		return decision
	}
	if strings.TrimSpace(os.Getenv("YAK_SSA_AST_BUILD_WINDOW_FILES")) != "" {
		decision.manualOverride = true
		return decision
	}

	decision.diagnosticsHeavy = c != nil && c.DiagnosticsEnabled() && diagnostics.GetLevel() == diagnostics.LevelLow
	decision.budgetBytes, decision.budgetSource = autoASTMemoryBudgetBytes()
	if decision.budgetBytes <= 0 {
		return decision
	}

	decision.slotCostBytes = astSlotCostBytes(c.GetLanguage())
	decision.window = clampASTBuildWindow(int(decision.budgetBytes/decision.slotCostBytes), concurrency)
	return decision
}

func parseGCPercentEnv(raw string) (int, bool) {
	normalized := strings.TrimSpace(strings.ToLower(raw))
	switch normalized {
	case "", "auto":
		return 0, false
	case "0", "false", "no", "off", "disable", "disabled":
		return -1, true
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return v, true
}

func (c *Config) resolveLargeProjectGCPercent() largeProjectGCDecision {
	decision := largeProjectGCDecision{
		largeProject: c != nil && c.GetCompileProjectBytes() >= largeProjectByteCap,
	}
	if !decision.largeProject || c == nil || !languagePreHandlerBuildsFiles(c.GetLanguage()) {
		return decision
	}
	if raw := strings.TrimSpace(os.Getenv("GOGC")); raw != "" {
		decision.manualOverride = true
		decision.source = "env:GOGC"
		return decision
	}
	if raw := strings.TrimSpace(os.Getenv("YAK_SSA_GC_PERCENT")); raw != "" {
		percent, ok := parseGCPercentEnv(raw)
		if !ok {
			log.Warnf("invalid YAK_SSA_GC_PERCENT=%q, using %d for large SSA project", raw, defaultLargeProjectGC)
		} else if percent <= 0 {
			decision.manualOverride = true
			decision.source = "env:YAK_SSA_GC_PERCENT"
			return decision
		} else {
			decision.percent = percent
			decision.source = "env:YAK_SSA_GC_PERCENT"
			return decision
		}
	}
	decision.percent = defaultLargeProjectGC
	decision.source = "auto:large-project"
	return decision
}

func (c *Config) applyLargeProjectGCPercent() func() {
	decision := c.resolveLargeProjectGCPercent()
	if !decision.largeProject {
		return nil
	}
	if decision.manualOverride {
		log.Infof("[ssa-compile] large project GC percent controlled by %s", decision.source)
		return nil
	}
	if decision.percent <= 0 {
		return nil
	}
	previous := debug.SetGCPercent(decision.percent)
	log.Infof(
		"[ssa-compile] large project GC percent=%d (previous=%d source=%s), restore after compile",
		decision.percent,
		previous,
		decision.source,
	)
	return func() {
		debug.SetGCPercent(previous)
	}
}

type antlrWorkerState struct {
	cache           *ssa.AntlrCache
	filesParsed     int
	bytesSinceReset int64
}

type antlrASTParseWorker struct {
	language        ssa.PreHandlerAnalyzer
	languageName    string
	resetEveryFiles int
	resetEveryBytes int64
}

func newAntlrASTParseWorker(c *Config) *antlrASTParseWorker {
	if c == nil {
		return &antlrASTParseWorker{}
	}
	resetEveryFiles := antlrCacheResetEveryFiles()
	resetEveryBytes := antlrCacheResetEveryBytes()
	if c.GetCompileProjectBytes() >= largeProjectByteCap {
		if _, ok := os.LookupEnv("YAK_ANTLR_CACHE_RESET_FILES"); !ok {
			resetEveryFiles = 1
		}
		if _, ok := os.LookupEnv("YAK_ANTLR_CACHE_RESET_BYTES"); !ok {
			resetEveryBytes = 2 * 1024 * 1024
		}
	}
	return &antlrASTParseWorker{
		language:        c.LanguageBuilder,
		languageName:    string(c.GetLanguage()),
		resetEveryFiles: resetEveryFiles,
		resetEveryBytes: resetEveryBytes,
	}
}

func (p *antlrASTParseWorker) initWorker() *utils.SafeMap[any] {
	store := utils.NewSafeMap[any]()
	store.Set(antlrWorkerStateKey, &antlrWorkerState{
		cache: p.newWorkerCache(),
	})
	return store
}

func (p *antlrASTParseWorker) newWorkerCache() *ssa.AntlrCache {
	if p == nil || p.language == nil || !ssa.WorkerAntlrCacheEnabled() {
		return nil
	}
	return p.language.GetAntlrCache()
}

func (p *antlrASTParseWorker) workerState(store *utils.SafeMap[any]) *antlrWorkerState {
	if store == nil {
		return &antlrWorkerState{
			cache: p.newWorkerCache(),
		}
	}

	if raw, ok := store.Get(antlrWorkerStateKey); ok {
		if state, ok := raw.(*antlrWorkerState); ok && state != nil {
			return state
		}
	}

	state := &antlrWorkerState{
		cache: p.newWorkerCache(),
	}
	store.Set(antlrWorkerStateKey, state)
	return state
}

func (p *antlrASTParseWorker) parseFileAST(path string, source string, store *utils.SafeMap[any]) (ssa.FrontAST, error) {
	if p == nil || p.language == nil {
		return nil, utils.Errorf("not select language %s", p.languageName)
	}
	if !p.language.FilterParseAST(path) {
		log.Debugf("skip parse ast file: %s filter by %s", path, p.languageName)
		return nil, nil
	}

	state := p.workerState(store)
	ast, err := p.language.ParseAST(source, state.cache)
	if state.cache == nil || (p.resetEveryFiles <= 0 && p.resetEveryBytes <= 0) {
		return ast, err
	}
	state.filesParsed++
	state.bytesSinceReset += int64(len(source))
	resetByFiles := p.resetEveryFiles > 0 && state.filesParsed%p.resetEveryFiles == 0
	resetByBytes := p.resetEveryBytes > 0 && state.bytesSinceReset >= p.resetEveryBytes
	if resetByFiles || resetByBytes {
		state.cache.ResetRuntimeCaches()
		state.bytesSinceReset = 0
	}
	if err != nil {
		log.Debugf("parsed file[%s] parse [%s]AST error[%s]", path, p.languageName, err)
	}
	return ast, err
}

func (c *Config) GetFileHandler(
	filesystem filesys_interface.FileSystem,
	preHandlerFiles []string,
	handlerFilesMap map[string]struct{},
) <-chan *ssareducer.FileContent {
	parser := newAntlrASTParseWorker(c)
	parse := func(path string, content []byte, store *utils.SafeMap[any]) (ssa.FrontAST, error) {
		start := time.Now()
		defer func() {
			log.Debugf("pre-handler cost:%v parse ast: %s; size(%s)", time.Since(start), path, formatFileSize(len(content)))
		}()

		defer func() {
			if r := recover(); r != nil {
				log.Errorf("pre-handler parse [%s] error %v  ", path, r)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		if _, needBuild := handlerFilesMap[path]; !needBuild {
			// don't need parse ast
			return nil, nil
		}

		return parser.parseFileAST(path, utils.UnsafeBytesToString(content), store)
	}
	initWorker := func() *utils.SafeMap[any] {
		return parser.initWorker()
	}
	concurrency := int(c.GetCompileConcurrency())
	astBuildWindow := c.resolveASTBuildWindow(concurrency)
	if astBuildWindow.largeProject {
		if astBuildWindow.manualOverride {
			log.Infof(
				"[ssa-compile] large project detected (%s), AST build window controlled by YAK_SSA_AST_BUILD_WINDOW_FILES and ANTLR cache reset files=%d bytes=%s",
				formatFileSize(int(c.GetCompileProjectBytes())),
				parser.resetEveryFiles,
				formatFileSize(int(parser.resetEveryBytes)),
			)
		} else if astBuildWindow.window > 0 {
			log.Infof(
				"[ssa-compile] large project detected (%s), auto AST build window=%d (language=%s diagnostics_heavy=%v budget=%s source=%s slot_cost=%s concurrency=%d), ANTLR cache reset files=%d bytes=%s",
				formatFileSize(int(c.GetCompileProjectBytes())),
				astBuildWindow.window,
				c.GetLanguage(),
				astBuildWindow.diagnosticsHeavy,
				formatFileSize(int(astBuildWindow.budgetBytes)),
				astBuildWindow.budgetSource,
				formatFileSize(int(astBuildWindow.slotCostBytes)),
				concurrency,
				parser.resetEveryFiles,
				formatFileSize(int(parser.resetEveryBytes)),
			)
		} else {
			log.Infof(
				"[ssa-compile] large project detected (%s), AST build window uses compile concurrency=%d (memory budget unavailable), ANTLR cache reset files=%d bytes=%s",
				formatFileSize(int(c.GetCompileProjectBytes())),
				concurrency,
				parser.resetEveryFiles,
				formatFileSize(int(parser.resetEveryBytes)),
			)
		}
	}
	return ssareducer.FilesHandler(
		c.ctx, filesystem, preHandlerFiles,
		parse, initWorker,
		c.GetCompileASTSequence(),
		concurrency,
		astBuildWindow.window,
	)
}

func formatFileSize(size int) string {
	if size < 1024 {
		return strconv.Itoa(size) + "B"
	}
	sizeKB := float64(size) / 1024.0
	if sizeKB < 1024 {
		return strconv.FormatFloat(sizeKB, 'f', 2, 64) + "KB"
	}
	sizeMB := sizeKB / 1024.0
	if sizeMB < 1024 {
		return strconv.FormatFloat(sizeMB, 'f', 2, 64) + "MB"
	}
	sizeGB := sizeMB / 1024.0
	return strconv.FormatFloat(sizeGB, 'f', 2, 64) + "GB"
}

// Size formats bytes into a human-readable string.
// Kept for backward compatibility across packages/tests.
func Size(size int) string {
	return formatFileSize(size)
}

type ScanResult struct {
	HandlerFiles    []string
	PreHandlerFiles []string
	HandlerFilesMap map[string]struct{}
	Folders         [][]string
	HandlerTotal    int
	PreHandlerTotal int
	// HandlerBytes is the total source byte size of files that enter the compile
	// stage. It is used to choose adaptive IR cache defaults for small vs large
	// projects; it is not persisted as part of user-facing project metadata.
	HandlerBytes int64
}

type ScanConfig struct {
	ProgramName     string
	ProgramPath     string
	FileSystem      filesys_interface.FileSystem
	ExcludeFunc     func(string) bool
	CheckLanguage   func(string) error
	CheckPreHandler func(string) error
	Context         context.Context
}

// ScanProjectFiles scans the project directory and returns the files to be processed
func ScanProjectFiles(cfg ScanConfig) (*ScanResult, error) {
	result := &ScanResult{
		HandlerFiles:    make([]string, 0),
		PreHandlerFiles: make([]string, 0),
		HandlerFilesMap: make(map[string]struct{}),
		Folders:         make([][]string, 0),
	}
	exclude := ssaconfig.ResolveCompileExcludeFunc(cfg.ExcludeFunc)

	err := filesys.Recursive(cfg.ProgramPath,
		filesys.WithFileSystem(cfg.FileSystem),
		filesys.WithContext(cfg.Context),
		filesys.WithDirStat(func(fullPath string, fi fs.FileInfo) error {
			if exclude(fullPath) {
				return filesys.SkipDir
			}

			folders := []string{cfg.ProgramName}
			// Use the filesystem's separator to split the path
			// Note: In the original code, this used c.fs.GetSeparators().
			// We should use cfg.FileSystem.GetSeparators() if it matches, or pass it in.
			// Assuming cfg.FileSystem is the one to use.
			sep := string(cfg.FileSystem.GetSeparators())
			folders = append(folders,
				strings.Split(fullPath, sep)...,
			)
			result.Folders = append(result.Folders, folders)
			return nil
		}),
		filesys.WithFileStat(func(path string, fi fs.FileInfo) error {
			if fi.Size() == 0 {
				return nil
			}
			if exclude(path) {
				return nil
			}
			if cfg.CheckLanguage != nil && cfg.CheckLanguage(path) == nil {
				result.HandlerTotal++
				result.HandlerBytes += fi.Size()
				result.HandlerFiles = append(result.HandlerFiles, path)
			}
			if cfg.CheckPreHandler != nil && cfg.CheckPreHandler(path) == nil {
				result.PreHandlerTotal++
				result.PreHandlerFiles = append(result.PreHandlerFiles, path)
				result.HandlerFilesMap[path] = struct{}{}
			}
			return nil
		}),
	)

	return result, err
}
