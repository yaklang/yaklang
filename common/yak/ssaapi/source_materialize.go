package ssaapi

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	jspparser "github.com/yaklang/yaklang/common/yak/java/jsp/parser"
	tj "github.com/yaklang/yaklang/common/yak/java/template2java"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
)

// sourceCacheFormatVersion must be bumped whenever the materialized source tree layout or
// the decompiler contract changes in a way that invalidates a previously written cache.
const sourceCacheFormatVersion = "v1"

// decompilerGateEnv lists the decompiler kill-switches that change the emitted source. They
// are part of the cache namespace so flipping one never serves sources produced under a
// different policy.
var decompilerGateEnv = []string{
	"JDEC_NO_ENUM_FOLD",
	"JDEC_NO_ENUM_SWITCH_FOLD",
	"JDEC_NO_SYN_BRIDGE_CTOR",
	"JDEC_NO_KEEP_DECLARED_CTOR",
	"JDEC_EXCEPTION_SENTINEL_DEGRADE_OFF",
	"JSR_INLINE_OFF",
}

// sourceMaterializer returns the per-compile decompiled-source materializer, creating it on
// first use. The materializer is the single place that turns an archive-backed code source
// into Java source: it decompiles every class exactly once, in parallel, and keeps the result
// in an in-memory VirtualFS that the planning and AST stages then read.
func (c *Config) sourceMaterializer() *filesys.SourceMaterializer {
	if c == nil {
		return nil
	}
	if c.materializedSources != nil {
		return c.materializedSources
	}
	var opts []filesys.SourceMaterializerOption
	if dir := filesys.DefaultSourceCacheDir(); dir != "" {
		opts = append(opts, filesys.WithSourceMaterializerDisk(
			dir, c.sourceCacheNamespace(), filesys.DefaultSourceDiskBudget(),
		))
	}
	if dir := c.GetCompileMaterializeOut(); dir != "" {
		opts = append(opts, filesys.WithSourceMaterializerTree(dir))
	}
	c.materializedSources = filesys.NewSourceMaterializer(opts...)
	return c.materializedSources
}

// wrapMaterializedSources attaches the materializer to the compile filesystem. Every read of
// the wrapped filesystem is served from the decompiled tree; walks, Stat and archive metadata
// keep delegating to the original filesystem.
func (c *Config) wrapMaterializedSources(fsys fi.FileSystem) fi.FileSystem {
	if fsys == nil {
		return fsys
	}
	materializer := c.sourceMaterializer()
	if materializer == nil {
		return fsys
	}
	return filesys.WithMaterializedSources(fsys, materializer)
}

// sourceCacheNamespace fingerprints everything that changes the decompiled output: the cache
// format, the decompiler build, its gate flags and the code source itself.
func (c *Config) sourceCacheNamespace() string {
	var b strings.Builder
	b.WriteString(sourceCacheFormatVersion)
	b.WriteByte('\n')
	b.WriteString(javajiveModuleVersion())
	b.WriteByte('\n')
	gates := append([]string(nil), decompilerGateEnv...)
	sort.Strings(gates)
	for _, name := range gates {
		b.WriteString(name)
		b.WriteByte('=')
		b.WriteString(strings.TrimSpace(os.Getenv(name)))
		b.WriteByte('\n')
	}
	b.WriteString(c.codeSourceFingerprint())
	return b.String()
}

// javajiveModuleVersion reports the decompiler build stamped into this binary, so an upgraded
// javajive never reuses sources produced by an older one.
func javajiveModuleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return "unknown"
	}
	for _, dep := range info.Deps {
		if dep == nil || dep.Path != "github.com/yaklang/javajive" {
			continue
		}
		if dep.Replace != nil {
			return dep.Replace.Path + "@" + dep.Replace.Version
		}
		return dep.Version
	}
	return "unknown"
}

// codeSourceFingerprint identifies the scanned source tree. For local files and archives the
// size and mtime are part of the identity: a payload rewritten in place (same path, new
// bytes) must not reuse the previous tree.
func (c *Config) codeSourceFingerprint() string {
	src := c.GetCodeSource()
	if src == nil {
		if path := c.programPath; path != "" {
			return "path:" + path
		}
		return "program:" + c.GetProgramName()
	}
	var b strings.Builder
	b.WriteString("kind:")
	b.WriteString(string(src.Kind))
	b.WriteByte('\n')
	if src.LocalFile != "" {
		b.WriteString("file:")
		b.WriteString(filepath.Clean(src.LocalFile))
		b.WriteByte('\n')
		if info, err := os.Stat(src.LocalFile); err == nil {
			b.WriteString("size:")
			b.WriteString(strconv.FormatInt(info.Size(), 10))
			b.WriteString(";mtime:")
			b.WriteString(strconv.FormatInt(info.ModTime().UnixNano(), 10))
			b.WriteByte('\n')
		}
	}
	if src.URL != "" {
		b.WriteString("url:")
		b.WriteString(src.URL)
		b.WriteByte('\n')
	}
	if src.Commit != "" {
		b.WriteString("commit:")
		b.WriteString(src.Commit)
		b.WriteByte('\n')
	}
	if src.Branch != "" {
		b.WriteString("branch:")
		b.WriteString(src.Branch)
		b.WriteByte('\n')
	}
	return b.String()
}

// SourcePreparationTimings keeps serial phase wall times separate from summed
// worker service times. ArchiveReadWorkerSeconds includes I/O + decompilation;
// it is deliberately not mislabeled as pure decompiler CPU time.
type SourcePreparationTimings struct {
	SourceCount                      int     `json:"source_count"`
	TemplateCount                    int     `json:"template_count"`
	WalkSeconds                      float64 `json:"walk_seconds"`
	ArchivePrepareSeconds            float64 `json:"archive_prepare_seconds"`
	TemplatePrepareSeconds           float64 `json:"template_prepare_seconds"`
	TemplateTranslationWorkerSeconds float64 `json:"template_translation_worker_seconds"`
	TemplateJavaWorkerSeconds        float64 `json:"template_java_worker_seconds"`
	ArchiveReadWorkerSeconds         float64 `json:"archive_read_worker_seconds"`
	StoreWorkerSeconds               float64 `json:"store_worker_seconds"`
	DiskWriteWorkerSeconds           float64 `json:"disk_write_worker_seconds"`
	DiskWriteBytes                   int64   `json:"disk_write_bytes"`
	ReleaseSeconds                   float64 `json:"release_seconds"`
	TemplateErrors                   int64   `json:"template_errors"`
	WalkErrors                       int64   `json:"walk_errors"`
}

// archiveGroupKey is the archive that owns path. Nested archives use the
// innermost jar/war/ear/zip. A loose class or jsp is its own group.
func archiveGroupKey(path string) string {
	normalized := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	best := -1
	extLen := 0
	for _, ext := range []string{".jar/", ".war/", ".ear/", ".par/", ".zip/"} {
		if i := strings.LastIndex(normalized, ext); i > best {
			best = i
			extLen = len(ext) - 1
		}
	}
	if best < 0 {
		return path
	}
	return normalized[:best+extLen]
}

// stripeByArchive round-robins paths across archives. The walk lists every
// class of one jar before the next, so a small worker window would otherwise
// decompile a single jar while the rest wait.
func stripeByArchive(paths []string) []string {
	if len(paths) < 2 {
		return paths
	}
	groups := make(map[string][]string)
	order := make([]string, 0)
	for _, path := range paths {
		key := archiveGroupKey(path)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], path)
	}
	if len(order) < 2 {
		return paths
	}
	out := make([]string, 0, len(paths))
	for {
		advanced := false
		for _, key := range order {
			group := groups[key]
			if len(group) == 0 {
				continue
			}
			out = append(out, group[0])
			groups[key] = group[1:]
			advanced = true
		}
		if !advanced {
			break
		}
	}
	return out
}

// prepareProjectSources runs before the source scan. Jar/zip entries (already
// expanded by the language filesystem), .class files and .jsp/.jspx files are
// independent file transforms: each class or archive member decompiles into one
// source, each JSP becomes one generated Java file. The pool uses the full
// compile concurrency. ScanProjectFiles then walks a tree whose reads are
// already source.
func (c *Config) prepareProjectSources(
	filesystem fi.FileSystem,
	reportProcess func(float64, string, ...any),
	storeProcess func(float64),
) (timings SourcePreparationTimings) {
	if c == nil || filesystem == nil {
		return
	}
	materializer := filesys.MaterializerOf(filesystem)
	if materializer == nil {
		return
	}
	var sources, templates []string
	walkStarted := time.Now()
	err := filesys.Recursive(c.programPath,
		filesys.WithFileSystem(filesystem),
		filesys.WithContext(c.GetContext()),
		filesys.WithDirStat(func(path string, info fs.FileInfo) error {
			if c.excludeMatcher != nil && c.excludeMatcher(path) {
				return filesys.SkipDir
			}
			return nil
		}),
		filesys.WithFileStat(func(path string, info fs.FileInfo) error {
			if info == nil || info.Size() == 0 {
				return nil
			}
			if c.excludeMatcher != nil && c.excludeMatcher(path) {
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".java", ".class":
				sources = append(sources, path)
			case ".jsp", ".jspx":
				templates = append(templates, path)
			}
			return nil
		}),
	)
	timings.WalkSeconds = time.Since(walkStarted).Seconds()
	timings.SourceCount, timings.TemplateCount = len(sources), len(templates)
	if err != nil {
		timings.WalkErrors++
		log.Warnf("[ssa-compile] source prepare walk: %v", err)
	}
	total := len(sources) + len(templates)
	if total == 0 {
		return
	}
	workers := filesys.PrefetchWorkers(c.GetCompileConcurrency())
	start := time.Now()
	log.Infof("[ssa-compile] source prepare start class/java=%d jsp=%d workers=%d", len(sources), len(templates), workers)

	var done atomic.Int64
	var reportMu sync.Mutex
	var lastReported int
	report := func() {
		n := int(done.Add(1))
		if reportProcess == nil {
			return
		}
		step := total / 100
		if step < 1 {
			step = 1
		}
		reportMu.Lock()
		if n != total && n-lastReported < step {
			reportMu.Unlock()
			return
		}
		lastReported = n
		reportMu.Unlock()
		p := sourcePrefetchProcessEnd * (float64(n) / float64(total))
		if storeProcess != nil {
			storeProcess(p)
		}
		reportProcess(p, fmt.Sprintf("[%s] prepare sources jar/zip/class/jsp (%d/%d)", "f0_prepare", n, total))
	}

	// Classes inside one jar are consecutive after the walk. A worker window
	// then sits inside a single jar. Round-robin puts several jars in flight
	// together. JSP conversion does not depend on decompilation, so it runs
	// on the same pool instead of waiting for every class.
	sources = stripeByArchive(sources)
	var prep sync.WaitGroup
	var archiveSeconds float64
	var prefetchErrors int64
	var templateSeconds, templateTranslation, templateJava float64
	var templateErrors int64
	if len(sources) > 0 {
		prep.Add(1)
		go func() {
			defer prep.Done()
			archiveStarted := time.Now()
			if err := materializer.Prefetch(c.GetContext(), filesystem, sources, workers, func(_, _ int) { report() }); err != nil {
				prefetchErrors++
				log.Warnf("[ssa-compile] source prepare prefetch: %v", err)
			}
			archiveSeconds = time.Since(archiveStarted).Seconds()
		}()
	}
	templateWorkers := workers / 4
	if templateWorkers < 1 {
		templateWorkers = workers
	}
	if templateWorkers > 8 {
		templateWorkers = 8
	}
	if len(templates) > 0 {
		prep.Add(1)
		go func() {
			defer prep.Done()
			templatesStarted := time.Now()
			templateTranslation, templateJava, templateErrors = c.prepareTemplates(filesystem, materializer, templates, templateWorkers, report)
			templateSeconds = time.Since(templatesStarted).Seconds()
		}()
	}
	prep.Wait()
	timings.ArchivePrepareSeconds = archiveSeconds
	timings.WalkErrors += prefetchErrors
	timings.TemplatePrepareSeconds = templateSeconds
	timings.TemplateTranslationWorkerSeconds = templateTranslation
	timings.TemplateJavaWorkerSeconds = templateJava
	timings.TemplateErrors = templateErrors
	releaseStarted := time.Now()
	filesys.ReleasePreparedSourceCaches(filesystem)
	// The virtual filesystem now holds the generated Java. JSP prediction state
	// must not survive into prehandler: the shared DFA only grows, and the
	// prepare allocation peak stays mapped until the heap doubles.
	c.releasePreparedJSPRuntime()
	timings.ReleaseSeconds = time.Since(releaseStarted).Seconds()

	stats := materializer.Stats()
	timings.ArchiveReadWorkerSeconds = stats.BaseReadSeconds
	timings.StoreWorkerSeconds = stats.StoreSeconds
	timings.DiskWriteWorkerSeconds, timings.DiskWriteBytes = stats.DiskWriteSeconds, stats.DiskWriteBytes
	log.Infof("[ssa-compile] source prepare finish cost=%v %s", time.Since(start), filesys.FormatSourceMaterializerStats(stats))
	if reportProcess != nil {
		reportProcess(sourcePrefetchProcessEnd, fmt.Sprintf(
			"[f0_prepare] prepared %d sources (%d jsp) in %v (%s)",
			stats.Paths, len(templates), time.Since(start).Round(time.Millisecond), filesys.FormatSourceMaterializerStats(stats),
		))
	}
	return timings
}

// jspJavaForSSA is the JSP side of the java2ssa contract: structured translation
// first, and the grammar-free translation when that Java does not parse. cache is
// the per-worker Java parser cache; without one every parse would use the
// package-shared DFA tables and keep every state it ever explored.
func jspJavaForSSA(content, path string, cache *ssa.AntlrCache) (tl.TemplateGeneratedInfo, error) {
	info, _, _, err := jspJavaForSSAWithTiming(content, path, cache)
	return info, err
}

func jspJavaForSSAWithTiming(content, path string, cache *ssa.AntlrCache) (info tl.TemplateGeneratedInfo, translation, javaParse time.Duration, err error) {
	parses := func(src string) bool {
		started := time.Now()
		ok := javaSourceParses(src, cache)
		javaParse += time.Since(started)
		return ok
	}
	started := time.Now()
	info, err = tj.ConvertTemplateToJava(tj.JSP, content, path)
	translation += time.Since(started)
	if err == nil && info != nil && parses(info.GetContent()) {
		return info, translation, javaParse, nil
	}
	started = time.Now()
	linear, lerr := tj.ConvertJSPLinear(content, path)
	translation += time.Since(started)
	if lerr != nil {
		if err != nil {
			return nil, translation, javaParse, err
		}
		return nil, translation, javaParse, lerr
	}
	if linear == nil || !parses(linear.GetContent()) {
		if err != nil {
			return nil, translation, javaParse, err
		}
		return nil, translation, javaParse, fmt.Errorf("jsp %s did not produce java that java2ssa can parse", path)
	}
	return linear, translation, javaParse, nil
}

func (c *Config) rememberPreparedTemplate(path string, info tl.TemplateGeneratedInfo) {
	if c == nil || info == nil || path == "" {
		return
	}
	c.preparedTemplatesMu.Lock()
	defer c.preparedTemplatesMu.Unlock()
	if c.preparedTemplates == nil {
		c.preparedTemplates = map[string]tl.TemplateGeneratedInfo{}
	}
	c.preparedTemplates[path] = info
}

func (c *Config) applyPreparedTemplates(prog *ssa.Program) {
	if c == nil || prog == nil {
		return
	}
	c.preparedTemplatesMu.Lock()
	templates := c.preparedTemplates
	c.preparedTemplates = nil
	c.preparedTemplatesMu.Unlock()
	seen := map[any]struct{}{}
	for path, info := range templates {
		prog.SetTemplate(path, info)
		if info == nil {
			continue
		}
		// Range offsets belong to the original page, not the generated Java.
		// The database reloads a range by that editor's hash, so the page has
		// to be saved with the program or the reloaded call has a nil range.
		for _, rng := range info.GetRangeMap() {
			if rng == nil {
				continue
			}
			editor := rng.GetEditor()
			if editor == nil {
				continue
			}
			if _, ok := seen[editor]; ok {
				continue
			}
			seen[editor] = struct{}{}
			if editor.GetProgramName() == "" {
				editor.SetProgramName(prog.GetProgramName())
			}
			prog.SaveEditor(editor)
		}
	}
}

func javaSourceParses(src string, cache *ssa.AntlrCache) bool {
	if strings.TrimSpace(src) == "" {
		return false
	}
	// Use the same frontend the AST stage uses. FrontendRaw skips
	// decompiler-output normalization and rejected JSP pages that compile.
	_, err := java2ssa.Frontend(src, cache)
	return err == nil
}

// newJavaParseCache builds an isolated parser cache for one prepare worker.
func newJavaParseCache() *ssa.AntlrCache {
	builder, ok := java2ssa.CreateBuilder().(*java2ssa.SSABuilder)
	if !ok {
		return nil
	}
	return builder.GetAntlrCache()
}

// currentHeapInuseMB reports live heap for prepare-phase attribution logs. The
// counter is sampled, not a limit, so it stays cheap on the conversion path.
func currentHeapInuseMB() float64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.HeapInuse) / 1048576
}

func (c *Config) prepareTemplates(
	filesystem fi.FileSystem,
	materializer *filesys.SourceMaterializer,
	paths []string,
	workers int,
	report func(),
) (translationSeconds, javaSeconds float64, conversionErrors int64) {
	if len(paths) == 0 || materializer == nil {
		return
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(paths) {
		workers = len(paths)
	}
	jobs := make(chan string)
	var wg sync.WaitGroup
	var generatedBytes atomic.Int64
	var converted atomic.Int64
	var doneCount atomic.Int64
	var translationNS, javaNS, errors atomic.Int64
	started := time.Now()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache := newJavaParseCache()
			defer func() {
				if cache != nil {
					cache.Clear()
				}
			}()
			workerParses := 0
			for path := range jobs {
				if c.GetContext() != nil && c.GetContext().Err() != nil {
					report()
					continue
				}
				raw, err := materializer.ReadFile(filesystem, path)
				if err != nil {
					log.Debugf("[ssa-compile] read jsp %s: %v", path, err)
					report()
					continue
				}
				fileStart := time.Now()
				info, translated, parsed, err := jspJavaForSSAWithTiming(string(raw), path, cache)
				translationNS.Add(translated.Nanoseconds())
				javaNS.Add(parsed.Nanoseconds())
				elapsed := time.Since(fileStart)
				workerParses++
				if cache != nil && workerParses%50 == 0 {
					cache.ResetRuntimeCaches()
				}
				if err != nil {
					errors.Add(1)
					log.Warnf("[ssa-compile] prepare jsp %s: %v", path, err)
					report()
					continue
				}
				javaSource := info.GetContent()
				if elapsed > 2*time.Second {
					log.Warnf("[ssa-compile] slow jsp convert %s cost=%v source=%d generated=%d", path, elapsed, len(raw), len(javaSource))
				}
				c.rememberPreparedTemplate(path, info)
				materializer.Put(path, []byte(javaSource))
				converted.Add(1)
				generatedBytes.Add(int64(len(javaSource)))
				n := doneCount.Add(1)
				if n%100 == 0 || int(n) == len(paths) {
					log.Infof("[ssa-compile] jsp convert progress=%d/%d converted=%d generated=%.1fMB heap=%.1fMB cost=%v",
						n, len(paths), converted.Load(), float64(generatedBytes.Load())/1048576, currentHeapInuseMB(), time.Since(started))
				}
				report()
			}
		}()
	}
	for _, path := range paths {
		jobs <- path
	}
	close(jobs)
	wg.Wait()
	return float64(translationNS.Load()) / float64(time.Second), float64(javaNS.Load()) / float64(time.Second), errors.Load()
}

// releasePreparedJSPRuntime runs at the virtual-filesystem boundary, before
// prehandler. Generated Java is already stored; the JSP lexer/parser tables and
// any Java caches created while checking that Java are scratch. Drop them and
// return the prepare peak to the OS so prehandler does not start on top of it.
func (c *Config) releasePreparedJSPRuntime() {
	before := currentHeapInuseMB()
	if c != nil && c.LanguageBuilder != nil {
		c.LanguageBuilder.Clearup()
	}
	jspparser.ResetJSPRuntimeCaches()
	runtime.GC()
	debug.FreeOSMemory()
	log.Infof("[ssa-compile] jsp runtime released before prehandler heap %.1fMB -> %.1fMB", before, currentHeapInuseMB())
}

// MaterializeResult is the in-memory Java tree produced from jars, classes and JSP.
// VirtualFS is the only source store retained after preparation.
type MaterializeResult struct {
	VirtualFS *filesys.VirtualFS
	Files     int64
	Bytes     int64
	Errors    int64
	Elapsed   time.Duration
	HeapMB    float64
	Timings   SourcePreparationTimings
}

// MaterializeLocalProject turns a local Java project into one virtual filesystem of
// generated Java. It decompiles jar/class files and converts JSP, then drops the
// JSP parser tables. It does not compile SSA.
func MaterializeLocalProject(root string, concurrency int) (*MaterializeResult, error) {
	local := filesys.NewRelLocalFs(root)
	opts := []ssaconfig.Option{
		WithFileSystem(local),
		WithProgramPath("."),
		WithLanguage(ssaconfig.JAVA),
		ssaconfig.WithCompileDisableDefaultExcludes(true),
	}
	if concurrency > 0 {
		opts = append(opts, WithConcurrency(concurrency))
	}
	cfg, err := DefaultConfig(opts...)
	if err != nil {
		return nil, err
	}
	cfg.resolveLanguageBuilder(local)
	wrapped := cfg.swapLanguageFs(local)
	started := time.Now()
	timings := cfg.prepareProjectSources(wrapped, nil, nil)
	materializer := filesys.MaterializerOf(wrapped)
	if materializer == nil || materializer.VirtualFS() == nil {
		return nil, fmt.Errorf("prepare did not produce a virtual filesystem")
	}
	var files int64
	var nbytes int64
	err = filesys.Recursive(".",
		filesys.WithFileSystem(materializer.VirtualFS()),
		filesys.WithFileStat(func(path string, info fs.FileInfo) error {
			if info == nil || info.IsDir() {
				return nil
			}
			files++
			nbytes += info.Size()
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}
	runtime.GC()
	debug.FreeOSMemory()
	return &MaterializeResult{
		VirtualFS: materializer.VirtualFS(),
		Files:     files,
		Bytes:     nbytes,
		Errors:    materializer.Stats().Errors + timings.TemplateErrors + timings.WalkErrors,
		Timings:   timings,
		Elapsed:   time.Since(started),
		HeapMB:    currentHeapInuseMB(),
	}, nil
}

// prefetchCompileSources materializes every compile input before the compile-unit planning
// starts. The planning pass (package scan + dependency scan) and the AST pipeline all read
// the same files; without the prefetch those reads run the decompiler again, one class at a
// time, on the planning goroutine.
//
// Progress occupies the [0, sourcePrefetchProcessEnd] slice of the compile stage so the UI
// shows movement during the (previously silent) decompile phase.
func (c *Config) prefetchCompileSources(
	filesystem fi.FileSystem,
	files []string,
	reportProcess func(float64, string, ...any),
	storeProcess func(float64),
) {
	if c == nil || len(files) == 0 {
		return
	}
	materializer := filesys.MaterializerOf(filesystem)
	if materializer == nil {
		return
	}
	workers := filesys.PrefetchWorkers(c.GetCompileConcurrency())
	start := time.Now()
	log.Infof("[ssa-compile] source materialize start files=%d workers=%d", len(files), workers)

	var lastReported int
	report := func(done, total int) {
		if total <= 0 || reportProcess == nil {
			return
		}
		// Report at most ~100 times: a status event per file would flood the job stream.
		step := total / 100
		if step < 1 {
			step = 1
		}
		if done != total && done-lastReported < step {
			return
		}
		lastReported = done
		p := sourcePrefetchProcessEnd * (float64(done) / float64(total))
		if storeProcess != nil {
			storeProcess(p)
		}
		reportProcess(p, fmt.Sprintf("[%s] decompile sources(%d/%d)", "f0_materialize", done, total))
	}
	if err := materializer.Prefetch(c.GetContext(), filesystem, files, workers, report); err != nil {
		log.Warnf("[ssa-compile] source materialize interrupted: %v", err)
	}
	stats := materializer.Stats()
	log.Infof(
		"[ssa-compile] source materialize finish cost=%v %s",
		time.Since(start), filesys.FormatSourceMaterializerStats(stats),
	)
	if reportProcess != nil {
		reportProcess(sourcePrefetchProcessEnd, fmt.Sprintf(
			"[f0_materialize] decompiled %d sources in %v (%s)",
			stats.Paths, time.Since(start).Round(time.Millisecond), filesys.FormatSourceMaterializerStats(stats),
		))
	}
}

// materializedSourceStats exposes materializer counters for diagnostics and tests.
func (c *Config) materializedSourceStats() filesys.SourceMaterializerStats {
	if c == nil || c.materializedSources == nil {
		return filesys.SourceMaterializerStats{}
	}
	return c.materializedSources.Stats()
}
