package ssaapi

import (
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssautils"
)

// preparedSourceCache returns the compile's source view, creating it on first
// use. ssautils owns decompilation, page translation and the memory bound; the
// config only ties one instance to the compile lifetime.
func (c *Config) preparedSourceCache() *ssautils.PreparedSources {
	if c == nil {
		return nil
	}
	if c.preparedSources != nil {
		return c.preparedSources
	}
	c.preparedSources = ssautils.NewPreparedSources(c.GetCompileMemoryLimit(), "")
	c.AddCleanupFunc(func() { _ = c.preparedSources.Close() })
	return c.preparedSources
}

// prepareProjectSources runs one language-source preparation pass. The returned
// timings are phase measurements, not SSA work.
func (c *Config) prepareProjectSources(
	filesystem fi.FileSystem,
	reportProcess func(float64, string, ...any),
	storeProcess func(float64),
) ssautils.Timings {
	if c == nil || filesystem == nil {
		return ssautils.Timings{}
	}
	sources := c.preparedSourceCache()
	if sources == nil {
		return ssautils.Timings{}
	}
	return sources.Prepare(filesystem, ssautils.PrepareOptions{
		Context:        c.GetContext(),
		Root:           c.programPath,
		Concurrency:    c.GetCompileConcurrency(),
		MaterializeOut: c.GetCompileMaterializeOut(),
		Exclude:        c.excludeMatcher,
		Report:         reportProcess,
		StoreProgress:  storeProcess,
		ReleaseRuntime: func() {
			if c.LanguageBuilder != nil {
				c.LanguageBuilder.Clearup()
			}
		},
	})
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

// prefetchCompileSources materializes every compile input before unit planning
// starts, so planning and AST build read each source once instead of running
// the decompiler again per pass.
func (c *Config) prefetchCompileSources(
	filesystem fi.FileSystem,
	files []string,
	reportProcess func(float64, string, ...any),
	storeProcess func(float64),
) {
	if c == nil || len(files) == 0 {
		return
	}
	sources := c.preparedSourceCache()
	if sources == nil {
		return
	}
	workers := ssautils.PrefetchWorkers(c.GetCompileConcurrency())
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
	if err := sources.Prefetch(c.GetContext(), filesystem, files, workers, report); err != nil {
		log.Warnf("[ssa-compile] source materialize interrupted: %v", err)
	}
	stats := sources.Stats()
	log.Infof(
		"[ssa-compile] source materialize finish cost=%v %s",
		time.Since(start), filesys.FormatSpillFSStats(stats),
	)
	if reportProcess != nil {
		reportProcess(sourcePrefetchProcessEnd, fmt.Sprintf(
			"[f0_materialize] decompiled %d sources in %v (%s)",
			stats.Paths, time.Since(start).Round(time.Millisecond), filesys.FormatSpillFSStats(stats),
		))
	}
}
