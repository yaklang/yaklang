package java2ssa

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/javajive/classparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memorybudget"
	jspparser "github.com/yaklang/yaklang/common/yak/java/jsp/parser"
	tj "github.com/yaklang/yaklang/common/yak/java/template2java"
	"github.com/yaklang/yaklang/common/yak/ssa"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
)

// This file owns Java source preparation: what a Java project looks like before
// the SSA pipeline can read it as Java source.
//
// A walk sees an archive as one opaque file and a page as markup, so neither is
// a compilation unit. Preparation turns both into Java, keeps the result behind
// a read-through cache, and records the page ranges so a finding points at the
// page instead of the generated unit. It runs once per compile and every later
// stage reads the same prepared view.

// defaultPrepareMemoryLimit keeps half of the usable heap for prepared sources;
// the rest belongs to planning and IR. Bytes over the limit go to the
// preparation's private directory instead of failing the compile.
func defaultPrepareMemoryLimit() int64 {
	limit := memorybudget.Usable() / 2
	if limit < 256<<20 {
		limit = 256 << 20
	}
	return limit
}

// preparedSources is the compile's prepared view of a Java project: a
// read-through cache over the archive-expanded filesystem, plus the page range
// maps of the pages it generated.
type preparedSources struct {
	builder *SSABuilder
	cache   *filesys.FileCache

	mu        sync.Mutex
	templates map[string]tl.TemplateGeneratedInfo
	treeDir   string
	timings   ssa.ProjectPrepareTimings
}

var _ ssa.PreparedProject = (*preparedSources)(nil)

func newPreparedSources(builder *SSABuilder, memoryLimit int64, overflowParent string) *preparedSources {
	if memoryLimit <= 0 {
		memoryLimit = defaultPrepareMemoryLimit()
	}
	p := &preparedSources{builder: builder}
	opts := []filesys.FileCacheOption{
		filesys.WithFileCacheMemoryLimit(memoryLimit),
		filesys.WithFileCacheOverflowDir(overflowParent),
		filesys.WithFileCachePublisher(func(name string, data []byte) {
			p.publishTree(name, data)
		}),
	}
	p.cache = filesys.NewFileCache(opts...)
	return p
}

// Wrap serves reads from the prepared cache while keeping the tree shape of
// base. Wrapping is idempotent for the same cache.
func (p *preparedSources) Wrap(base fi.FileSystem) fi.FileSystem {
	if p == nil {
		return base
	}
	return filesys.WrapWithFileCache(base, p.cache)
}

// Close drops cached sources and the private overflow directory.
func (p *preparedSources) Close() error {
	if p == nil {
		return nil
	}
	return p.cache.Close()
}

// Stats reports cache counters.
func (p *preparedSources) Stats() ssa.ProjectPrepareStats {
	if p == nil {
		return ssa.ProjectPrepareStats{}
	}
	stats := p.cache.Stats()
	return ssa.ProjectPrepareStats{
		Paths:      stats.Paths,
		Hits:       stats.Hits,
		Misses:     stats.Misses,
		Errors:     stats.Errors,
		Prefetched: stats.Prefetched,
		MemoryHold: stats.MemoryHold,
		DiskHold:   stats.DiskHold,
	}
}

// Timings reports the phase measurements of the pass that produced these
// sources.
func (p *preparedSources) Timings() ssa.ProjectPrepareTimings {
	if p == nil {
		return ssa.ProjectPrepareTimings{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.timings
}

// WalkEntries visits every prepared path with its size. The callback may read
// from the prepared filesystem.
func (p *preparedSources) WalkEntries(fn func(name string, size int64) error) error {
	if p == nil {
		return nil
	}
	return p.cache.WalkEntries(fn)
}

// Templates returns the generated page metadata, keyed by the original path.
func (p *preparedSources) Templates() map[string]tl.TemplateGeneratedInfo {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]tl.TemplateGeneratedInfo, len(p.templates))
	for path, info := range p.templates {
		out[path] = info
	}
	return out
}

func (p *preparedSources) rememberTemplate(path string, info tl.TemplateGeneratedInfo) {
	if p == nil || info == nil || path == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.templates == nil {
		p.templates = make(map[string]tl.TemplateGeneratedInfo)
	}
	p.templates[path] = info
}

// Prefetch warms the cache for already-planned compile files.
func (p *preparedSources) Prefetch(ctx context.Context, base fi.FileSystem, paths []string, workers int, onProgress func(done, total int)) error {
	if p == nil {
		return nil
	}
	return p.cache.Prefetch(ctx, base, paths, workers, onProgress)
}

// prepare walks the expanded filesystem once and produces every source the
// compile will read: each archive member decompiled, each page translated.
func (p *preparedSources) prepare(base fi.FileSystem, opts ssa.ProjectPrepareOptions) ssa.ProjectPrepareTimings {
	var timings ssa.ProjectPrepareTimings
	if p == nil || base == nil {
		return timings
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.MaterializeOut != "" {
		p.SetMaterializeOut(opts.MaterializeOut)
	}

	var sources, templates []string
	walkStarted := time.Now()
	err := filesys.Recursive(opts.Root,
		filesys.WithFileSystem(base),
		filesys.WithContext(ctx),
		filesys.WithDirStat(func(path string, info fs.FileInfo) error {
			if opts.Exclude != nil && opts.Exclude(path) {
				return filesys.SkipDir
			}
			return nil
		}),
		filesys.WithFileStat(func(path string, info fs.FileInfo) error {
			if info == nil || info.Size() == 0 {
				return nil
			}
			if opts.Exclude != nil && opts.Exclude(path) {
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
		log.Warnf("[ssa-compile] java source prepare walk: %v", err)
	}
	total := len(sources) + len(templates)
	if total == 0 {
		return timings
	}

	workers := prepareWorkers(opts.Concurrency)
	start := time.Now()
	log.Infof("[ssa-compile] java source prepare start class/java=%d jsp=%d workers=%d", len(sources), len(templates), workers)

	var done atomic.Int64
	var reportMu sync.Mutex
	var lastReported int
	report := func(label string) {
		n := int(done.Add(1))
		if opts.Progress == nil {
			return
		}
		// Report at most ~100 times: one event per file would flood the job stream.
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
		opts.Progress(float64(n)/float64(total), fmt.Sprintf("%s (%d/%d)", label, n, total))
	}

	// Classes inside one archive are consecutive after the walk. A worker window
	// then sits inside a single archive; round-robin puts several in flight.
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
			if err := p.cache.Prefetch(ctx, base, sources, workers, func(_, _ int) {
				report("[f0_prepare] decompile sources")
			}); err != nil {
				prefetchErrors++
				log.Warnf("[ssa-compile] java source prepare prefetch: %v", err)
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
			templateTranslation, templateJava, templateErrors = p.prepareTemplates(ctx, base, templates, templateWorkers, report)
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
	filesys.ReleasePreparationCaches(base)
	// The cache now holds the generated Java. JSP prediction state must not
	// survive into prehandler: the shared DFA only grows.
	p.releaseJSPRuntime()
	timings.ReleaseSeconds = time.Since(releaseStarted).Seconds()

	stats := p.cache.Stats()
	timings.ArchiveReadWorkerSeconds = stats.BaseRead
	timings.StoreWorkerSeconds = stats.Store
	timings.DiskWriteWorkerSeconds, timings.DiskWriteBytes = stats.DiskWrite, stats.DiskHold
	log.Infof("[ssa-compile] java source prepare finish cost=%v %s", time.Since(start), filesys.FormatFileCacheStats(stats))
	if opts.Progress != nil {
		opts.Progress(1, fmt.Sprintf(
			"[f0_prepare] prepared %d sources (%d jsp) in %v (%s)",
			stats.Paths, len(templates), time.Since(start).Round(time.Millisecond), filesys.FormatFileCacheStats(stats),
		))
	}
	return timings
}

// PreparedProjectResult is the Java source tree produced from archives, class
// files and pages for a whole project.
type PreparedProjectResult struct {
	Sources ssa.PreparedProject
	FS      fi.FileSystem
	Files   int64
	Bytes   int64
	Errors  int64
	Elapsed time.Duration
	Timings ssa.ProjectPrepareTimings
}

// PrepareProjectSources turns a local Java project into one prepared
// filesystem. It decompiles jar/class files and converts JSP, then drops the
// parser tables. It does not compile SSA.
//
// This is the Java frontend's own entry point: a caller that wants a prepared
// Java tree asks Java for it instead of reimplementing the steps.
func PrepareProjectSources(root string, concurrency int) (*PreparedProjectResult, error) {
	local := filesys.NewRelLocalFs(root)
	builder, ok := CreateBuilder().(*SSABuilder)
	if !ok {
		return nil, utils.Errorf("java builder is unavailable")
	}
	wrapped := builder.WrapWithPreprocessedFS(local, true)
	sources := newPreparedSources(builder, 0, "")
	started := time.Now()
	timings := sources.prepare(wrapped, ssa.ProjectPrepareOptions{
		Root:        ".",
		Concurrency: concurrency,
	})
	fsys := sources.Wrap(wrapped)
	var files, nbytes int64
	if err := sources.WalkEntries(func(_ string, size int64) error {
		files++
		nbytes += size
		return nil
	}); err != nil {
		return nil, err
	}
	runtime.GC()
	debug.FreeOSMemory()
	stats := sources.Stats()
	return &PreparedProjectResult{
		Sources: sources,
		FS:      fsys,
		Files:   files,
		Bytes:   nbytes,
		Errors:  stats.Errors + timings.TemplateErrors + timings.WalkErrors,
		Elapsed: time.Since(started),
		Timings: timings,
	}, nil
}

func (p *preparedSources) prepareTemplates(
	ctx context.Context,
	base fi.FileSystem,
	paths []string,
	workers int,
	report func(string),
) (translationSeconds, javaSeconds float64, conversionErrors int64) {
	if len(paths) == 0 {
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
				if ctx.Err() != nil {
					report("[f0_prepare] translate pages")
					continue
				}
				// Read the raw page from the inner filesystem: only the generated
				// Java belongs in the cache and the materialize-out tree.
				raw, err := filesys.UnwrapFileCache(base, p.cache).ReadFile(path)
				if err != nil {
					log.Debugf("[ssa-compile] read jsp %s: %v", path, err)
					report("[f0_prepare] translate pages")
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
					report("[f0_prepare] translate pages")
					continue
				}
				javaSource := info.GetContent()
				if elapsed > 2*time.Second {
					log.Warnf("[ssa-compile] slow jsp convert %s cost=%v source=%d generated=%d", path, elapsed, len(raw), len(javaSource))
				}
				p.rememberTemplate(path, info)
				p.cache.Put(path, []byte(javaSource))
				converted.Add(1)
				generatedBytes.Add(int64(len(javaSource)))
				n := doneCount.Add(1)
				if n%100 == 0 || int(n) == len(paths) {
					log.Infof("[ssa-compile] jsp convert progress=%d/%d converted=%d generated=%.1fMB heap=%.1fMB cost=%v",
						n, len(paths), converted.Load(), float64(generatedBytes.Load())/1048576, currentHeapInuseMB(), time.Since(started))
				}
				report("[f0_prepare] translate pages")
			}
		}()
	}
	for _, path := range paths {
		select {
		case <-ctx.Done():
		case jobs <- path:
			continue
		}
		break
	}
	close(jobs)
	wg.Wait()
	return float64(translationNS.Load()) / float64(time.Second), float64(javaNS.Load()) / float64(time.Second), errors.Load()
}

// releaseJSPRuntime runs at the prepared-source boundary, before prehandler.
// Generated Java is already stored; JSP lexer/parser tables and Java caches
// created while checking that Java are scratch.
func (p *preparedSources) releaseJSPRuntime() {
	before := currentHeapInuseMB()
	if p.builder != nil {
		p.builder.Clearup()
	}
	jspparser.ResetJSPRuntimeCaches()
	runtime.GC()
	debug.FreeOSMemory()
	log.Infof("[ssa-compile] jsp runtime released before prehandler heap %.1fMB -> %.1fMB", before, currentHeapInuseMB())
}

// publishTree mirrors a prepared source into MaterializeOut. Non-template
// files keep the class-file name with a .java suffix; pages get page.jsp.java
// so they cannot collide with a real page.java.
func (p *preparedSources) publishTree(name string, data []byte) {
	if p == nil || len(data) == 0 {
		return
	}
	p.mu.Lock()
	dir := p.treeDir
	p.mu.Unlock()
	if dir == "" {
		return
	}
	rel, err := treeRel(name)
	if err != nil {
		log.Warnf("[source-materialize] %s: %v", name, err)
		return
	}
	target := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		log.Warnf("[source-materialize] create %s: %v", filepath.Dir(target), err)
		return
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		log.Warnf("[source-materialize] write %s: %v", target, err)
	}
}

// SetMaterializeOut enables writing the prepared tree.
func (p *preparedSources) SetMaterializeOut(dir string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.treeDir = strings.TrimSpace(dir)
	p.mu.Unlock()
}

// treeRel maps a virtual source path onto a .java file.
func treeRel(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Clean("/" + name)
	name = strings.TrimPrefix(name, "/")
	if name == "" || name == "." || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") {
		return "", fmt.Errorf("materialize path %q escapes the output tree", name)
	}
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".class"):
		name = name[:len(name)-len(".class")] + ".java"
	case strings.HasSuffix(lower, ".jsp"), strings.HasSuffix(lower, ".jspx"):
		name += ".java"
	}
	return name, nil
}

// prepareWorkers returns the worker count for source preparation.
// YAK_SSA_SOURCE_WORKERS overrides the compile concurrency.
func prepareWorkers(compileConcurrency int) int {
	if raw := strings.TrimSpace(os.Getenv("YAK_SSA_SOURCE_WORKERS")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			return v
		}
	}
	workers := compileConcurrency
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers < 1 {
		workers = 1
	}
	return workers
}

// archiveGroupKey is the archive that owns path. Nested archives use the
// innermost jar/war/ear/zip. A loose class or page is its own group.
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

// stripeByArchive round-robins paths across archives.
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

// jspJavaForSSAWithTiming translates a page and proves the result parses as
// Java. The grammar-based translation is preferred; the linear fallback covers
// pages the grammar rejects but Java can still parse.
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

func javaSourceParses(src string, cache *ssa.AntlrCache) bool {
	if strings.TrimSpace(src) == "" {
		return false
	}
	// Use the same frontend the AST stage uses.
	_, err := Frontend(src, cache)
	return err == nil
}

// newJavaParseCache builds an isolated parser cache for one prepare worker.
func newJavaParseCache() *ssa.AntlrCache {
	builder, ok := CreateBuilder().(*SSABuilder)
	if !ok {
		return nil
	}
	return builder.GetAntlrCache()
}

// currentHeapInuseMB reports live heap for prepare-phase attribution logs.
func currentHeapInuseMB() float64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.HeapInuse) / 1048576
}

// javaPrepareEvidence marks a file the Java pipeline must rewrite before a scan
// can read it: an archive to expand, a bytecode file to decompile, or a page to
// translate.
var javaPrepareEvidence = map[string]struct{}{
	".jar": {}, ".war": {}, ".ear": {},
	".class": {}, ".jsp": {}, ".jspx": {},
}

// NeedsProjectPreparation reports whether the tree holds inputs that must be
// rewritten before they are Java source. It stops at the first such file, so it
// costs one bounded walk and no read.
func (*SSABuilder) NeedsProjectPreparation(fsys fi.FileSystem, root string) bool {
	if fsys == nil {
		return false
	}
	found := false
	_ = filesys.Recursive(root,
		filesys.WithFileSystem(fsys),
		filesys.WithFileStat(func(path string, info fs.FileInfo) error {
			if info == nil || info.IsDir() {
				return nil
			}
			if _, ok := javaPrepareEvidence[strings.ToLower(filepath.Ext(path))]; ok {
				found = true
				return filesys.SkipAll
			}
			return nil
		}),
	)
	return found
}

// OpenArchiveCodeSource opens a jar/war code source as a filesystem whose
// classes read as Java source.
//
// The code source is the archive itself, so its classes must go through the jar
// reader: that reader resolves the declaration scope of every member (anonymous
// and enum bodies included). A plain zip view does not, and rejects classes
// whose constant-specific bodies then cannot be folded back.
func OpenArchiveCodeSource(local string, recursive bool) (fi.FileSystem, error) {
	jarFS, err := javaclassparser.NewJarFSFromLocalWithOptions(local, recursive)
	if err != nil {
		return nil, err
	}
	// SSA parses the rendered text. Keep a unique original member when javac
	// cannot recompile that spelling, instead of replacing the class with a
	// failure comment.
	jarFS.SetRetainUnprovedSource(true)
	return filesys.NewUnifiedFS(jarFS,
		filesys.WithUnifiedFsExtMap(".class", ".java"),
	), nil
}

// PrepareProject produces every Java source the compile will read, behind one
// read-through cache.
func (s *SSABuilder) PrepareProject(fsys fi.FileSystem, opts ssa.ProjectPrepareOptions) (ssa.PreparedProject, error) {
	if fsys == nil {
		return nil, utils.Errorf("prepare java project: nil filesystem")
	}
	sources := newPreparedSources(s, opts.MemoryLimit, "")
	timings := sources.prepare(fsys, opts)
	sources.mu.Lock()
	sources.timings = timings
	sources.mu.Unlock()
	return sources, nil
}
