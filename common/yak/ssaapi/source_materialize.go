package ssaapi

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	tj "github.com/yaklang/yaklang/common/yak/java/template2java"
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
) {
	if c == nil || filesystem == nil {
		return
	}
	materializer := filesys.MaterializerOf(filesystem)
	if materializer == nil {
		return
	}
	var sources, templates []string
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
	if err != nil {
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

	if len(sources) > 0 {
		_ = materializer.Prefetch(c.GetContext(), filesystem, sources, workers, func(_, _ int) { report() })
	}
	c.prepareTemplates(filesystem, materializer, templates, workers, report)

	stats := materializer.Stats()
	log.Infof("[ssa-compile] source prepare finish cost=%v %s", time.Since(start), filesys.FormatSourceMaterializerStats(stats))
	if reportProcess != nil {
		reportProcess(sourcePrefetchProcessEnd, fmt.Sprintf(
			"[f0_prepare] prepared %d sources (%d jsp) in %v (%s)",
			stats.Paths, len(templates), time.Since(start).Round(time.Millisecond), filesys.FormatSourceMaterializerStats(stats),
		))
	}
}

// jspJavaForSSA is the JSP side of the java2ssa contract: structured translation
// first, and the grammar-free translation when that Java does not parse.
func jspJavaForSSA(content, path string) (string, error) {
	info, err := tj.ConvertTemplateToJava(tj.JSP, content, path)
	if err == nil && info != nil && javaSourceParses(info.GetContent()) {
		return info.GetContent(), nil
	}
	linear, lerr := tj.ConvertJSPLinear(content, path)
	if lerr != nil {
		if err != nil {
			return "", err
		}
		return "", lerr
	}
	if linear == nil || !javaSourceParses(linear.GetContent()) {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("jsp %s did not produce java that java2ssa can parse", path)
	}
	return linear.GetContent(), nil
}

func javaSourceParses(src string) bool {
	if strings.TrimSpace(src) == "" {
		return false
	}
	_, err := java2ssa.Frontend(src)
	return err == nil
}

func (c *Config) prepareTemplates(
	filesystem fi.FileSystem,
	materializer *filesys.SourceMaterializer,
	paths []string,
	workers int,
	report func(),
) {
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
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
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
				javaSource, err := jspJavaForSSA(string(raw), path)
				if err != nil {
					log.Debugf("[ssa-compile] prepare jsp %s: %v", path, err)
					report()
					continue
				}
				materializer.Put(path, []byte(javaSource))
				report()
			}
		}()
	}
	for _, path := range paths {
		jobs <- path
	}
	close(jobs)
	wg.Wait()
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
