package ssa

import (
	"context"
	"fmt"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
)

// ProjectPrepareOptions describes one whole-project preparation pass. The zero
// value means: whole tree, machine concurrency, frontend default memory bound,
// no excludes and no materialize output.
type ProjectPrepareOptions struct {
	Context     context.Context
	Root        string
	Concurrency int
	// MemoryLimit bounds the prepared bytes kept in memory; the rest goes to
	// the preparation's private directory. <= 0 uses the frontend default.
	MemoryLimit int64
	// MaterializeOut writes every prepared source under a real tree.
	MaterializeOut string
	// Exclude prunes directories and files from preparation.
	Exclude func(string) bool
	// Progress reports this phase's own progress: fraction is 0..1 and the
	// caller maps it onto the compile's progress budget.
	Progress func(fraction float64, message string)
}

// ProjectPrepareTimings keeps serial phase wall times separate from summed
// worker service times. ArchiveRead includes I/O and decompilation; it is not
// pure CPU time.
type ProjectPrepareTimings struct {
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

// ProjectPrepareStats is the preparation's own accounting.
type ProjectPrepareStats struct {
	Paths      int64
	Hits       int64
	Misses     int64
	Errors     int64
	Prefetched int64
	// MemoryHold and DiskHold are the retained bytes in each place.
	MemoryHold int64
	DiskHold   int64
}

// String renders the counters for a log line.
func (s ProjectPrepareStats) String() string {
	return fmt.Sprintf(
		"cached=%d hits=%d misses=%d prefetched=%d errors=%d memory=%.1fMB disk=%.1fMB",
		s.Paths, s.Hits, s.Misses, s.Prefetched, s.Errors,
		float64(s.MemoryHold)/1024/1024, float64(s.DiskHold)/1024/1024,
	)
}

// PreparedProject is the source view a frontend produced for one compile.
type PreparedProject interface {
	// Wrap serves reads from the prepared sources while keeping the tree
	// shape of base.
	Wrap(base fi.FileSystem) fi.FileSystem
	// WalkEntries visits every prepared path with its size.
	WalkEntries(fn func(name string, size int64) error) error
	// Templates returns generated template metadata keyed by the original
	// path, so a finding points at the page instead of the generated unit.
	Templates() map[string]tl.TemplateGeneratedInfo
	Stats() ProjectPrepareStats
	// Timings reports the phase measurements of the pass that produced these
	// sources.
	Timings() ProjectPrepareTimings
	Close() error
}

// ProjectPreparer is implemented by a frontend whose compile inputs must be
// rewritten before the pipeline can read them as source. Java is the case: an
// archive is one opaque file to a walk, and a page is markup, not a
// compilation unit. A frontend without such a step does not implement this, so
// the pipeline holds no per-language branch.
type ProjectPreparer interface {
	// NeedsProjectPreparation reports whether fsys holds inputs this frontend
	// must rewrite first. It samples the tree and must stay cheap.
	NeedsProjectPreparation(fsys fi.FileSystem, root string) bool
	// PrepareProject produces every source the compile will read.
	PrepareProject(fsys fi.FileSystem, opts ProjectPrepareOptions) (PreparedProject, error)
}
