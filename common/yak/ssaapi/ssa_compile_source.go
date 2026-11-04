package ssaapi

import (
	"context"
	"fmt"
	"runtime"
	"time"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// This file ties one prepared-source view to one compile. The language frontend
// owns what preparation means; this file only calls it and maps its progress
// onto the compile's own progress budget.

// prepareProjectSources runs the frontend's source-preparation pass, if the
// frontend has one and the tree needs it. A frontend without a preparation step
// leaves the filesystem untouched.
func (c *Config) prepareProjectSources(
	filesystem fi.FileSystem,
	reportProcess func(float64, string, ...any),
	storeProcess func(float64),
) ssa.ProjectPrepareTimings {
	if c == nil || filesystem == nil || c.LanguageBuilder == nil {
		return ssa.ProjectPrepareTimings{}
	}
	preparer, ok := c.LanguageBuilder.(ssa.ProjectPreparer)
	if !ok {
		return ssa.ProjectPrepareTimings{}
	}
	// A materialize-out run always prepares: writing the tree IS the output.
	if c.GetCompileMaterializeOut() == "" && !preparer.NeedsProjectPreparation(filesystem, c.programPath) {
		return ssa.ProjectPrepareTimings{}
	}
	opts := ssa.ProjectPrepareOptions{
		Context:        c.GetContext(),
		Root:           c.programPath,
		Concurrency:    c.GetCompileConcurrency(),
		MemoryLimit:    c.GetCompileMemoryLimit(),
		MaterializeOut: c.GetCompileMaterializeOut(),
		Exclude:        c.excludeMatcher,
	}
	opts.Progress = func(fraction float64, message string) {
		if fraction < 0 {
			fraction = 0
		}
		if fraction > 1 {
			fraction = 1
		}
		p := sourcePrefetchProcessEnd * fraction
		if storeProcess != nil {
			storeProcess(p)
		}
		if reportProcess != nil {
			reportProcess(p, "[%s] %s", "f0_prepare", message)
		}
	}
	sources, err := preparer.PrepareProject(filesystem, opts)
	if err != nil {
		log.Warnf("[ssa-compile] source prepare failed: %v", err)
		return ssa.ProjectPrepareTimings{WalkErrors: 1}
	}
	if sources == nil {
		return ssa.ProjectPrepareTimings{}
	}
	c.preparedSources = sources
	c.AddCleanupFunc(func() { _ = sources.Close() })
	return sources.Timings()
}

// applyPreparedTemplates copies the page range maps onto the program before
// build. Without them a finding points at the synthetic generated method
// instead of the original scriptlet.
func (c *Config) applyPreparedTemplates(prog *ssa.Program) {
	if c == nil || prog == nil || c.preparedSources == nil {
		return
	}
	seen := map[any]struct{}{}
	for path, info := range c.preparedSources.Templates() {
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

// prefetchCompileSources materializes compile inputs the prepared view has not
// produced yet, so planning and AST build read each source once. The prepared
// view already holds most of them.
func (c *Config) prefetchCompileSources(
	filesystem fi.FileSystem,
	files []string,
	reportProcess func(float64, string, ...any),
	storeProcess func(float64),
) {
	if c == nil || len(files) == 0 {
		return
	}
	sources := c.preparedSources
	if sources == nil {
		return
	}
	workers := preparePrefetchWorkers(c.GetCompileConcurrency())
	start := time.Now()
	log.Infof("[ssa-compile] source prefetch start files=%d workers=%d", len(files), workers)

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
	if prefetcher, ok := sources.(interface {
		Prefetch(ctx context.Context, base fi.FileSystem, paths []string, workers int, onProgress func(done, total int)) error
	}); ok {
		if err := prefetcher.Prefetch(c.GetContext(), filesystem, files, workers, report); err != nil {
			log.Warnf("[ssa-compile] source prefetch interrupted: %v", err)
		}
	}
	stats := sources.Stats()
	log.Infof("[ssa-compile] source prefetch finish cost=%v %s", time.Since(start), stats)
	if reportProcess != nil {
		reportProcess(sourcePrefetchProcessEnd, fmt.Sprintf(
			"[f0_materialize] decompiled %d sources in %v (%s)",
			stats.Paths, time.Since(start).Round(time.Millisecond), stats,
		))
	}
}

// preparePrefetchWorkers sizes the prefetch pass. YAK_SSA_SOURCE_WORKERS
// overrides the compile concurrency.
func preparePrefetchWorkers(compileConcurrency int) int {
	workers := compileConcurrency
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers < 1 {
		return 1
	}
	return workers
}
