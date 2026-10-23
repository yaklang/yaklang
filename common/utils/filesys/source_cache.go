package filesys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memorybudget"
)

// SourceMaterializer decompiles an archive-backed code source once, in
// parallel, and keeps the results in an in-memory VirtualFS.
//
// Why: reading a `.class` entry through the expanded jar filesystem runs the
// decompiler every time. A single SSA compile reads each source three times
// (package scan for compile units, dependency scan, AST build), and compile
// planning did that on one goroutine, so a large Java source paid for 3x
// serial decompilation. Decompiling each entry is independent, so it is both
// parallelizable and memoizable.
//
// The materializer is the memo: every read goes through ReadFile, which
// returns the in-memory copy when present and otherwise decompiles exactly
// once (concurrent callers for the same path wait for the first load).
// Prefetch warms the whole tree with a worker pool before the compile stages
// start, so planning and AST build read from memory only.
//
// Sources are kept in RAM for the lifetime of the scan. That is the intended
// trade-off: a decompiled tree is a few hundred MB for a large Java project,
// far less than re-running the decompiler three times.
type SourceMaterializer struct {
	overlay   *VirtualFS
	overlayMu sync.Mutex // VirtualFS is not safe for concurrent use
	pathCount atomic.Int64

	// disk tier is optional (cross-run reuse). Disabled by default.
	diskDir   string
	maxDisk   int64
	diskBytes atomic.Int64

	// treeDir is a real directory of .java files. dropMemory means those
	// files are the source of truth: the overlay does not keep their bytes.
	treeDir    string
	dropMemory bool
	treeSeen   map[string]int64

	mu       sync.Mutex
	inflight map[string]*materializeCall

	hits        atomic.Int64
	misses      atomic.Int64
	diskHits    atomic.Int64
	errors      atomic.Int64
	loadedB     atomic.Int64
	prefetched  atomic.Int64
	baseReadNS  atomic.Int64
	storeNS     atomic.Int64
	diskWriteNS atomic.Int64
}

type materializeCall struct {
	done chan struct{}
	data []byte
	err  error
}

// SourceMaterializerOption configures a SourceMaterializer.
type SourceMaterializerOption func(*SourceMaterializer)

// WithSourceMaterializerDisk enables a write-through disk tier so a later scan
// of the same source fingerprint can skip decompilation entirely. namespace
// must change whenever the decompiled output would change (source fingerprint
// plus decompiler gates); maxBytes <= 0 means unbounded for this process.
func WithSourceMaterializerDisk(dir, namespace string, maxBytes int64) SourceMaterializerOption {
	return func(m *SourceMaterializer) {
		dir = strings.TrimSpace(dir)
		namespace = strings.TrimSpace(namespace)
		if dir == "" || namespace == "" {
			return
		}
		sum := sha256.Sum256([]byte(namespace))
		m.diskDir = filepath.Join(dir, hex.EncodeToString(sum[:])[:32])
		m.maxDisk = maxBytes
	}
}

// WithSourceMaterializerTree writes each materialized source into dir as a
// real file and does not retain the bytes in memory. A later compile can
// read that directory without the decompiler.
func WithSourceMaterializerTree(dir string) SourceMaterializerOption {
	return func(m *SourceMaterializer) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return
		}
		m.treeDir = dir
		m.dropMemory = true
	}
}

// NewSourceMaterializer creates an empty materializer.
func NewSourceMaterializer(opts ...SourceMaterializerOption) *SourceMaterializer {
	m := &SourceMaterializer{
		overlay:  NewVirtualFs(),
		inflight: make(map[string]*materializeCall),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}
	return m
}

// VirtualFS returns the in-memory tree holding the decompiled sources. It is
// safe to read; callers must not mutate it while a compile is running.
func (m *SourceMaterializer) VirtualFS() *VirtualFS {
	if m == nil {
		return nil
	}
	return m.overlay
}

// Has reports whether the path is already materialized in memory.
func (m *SourceMaterializer) Has(name string) bool {
	if m == nil || m.overlay == nil {
		return false
	}
	m.overlayMu.Lock()
	defer m.overlayMu.Unlock()
	_, err := m.overlay.Stat(name)
	return err == nil
}

// ReadFile returns the materialized source for name, decompiling it exactly
// once on a miss. base is only consulted on a miss.
func (m *SourceMaterializer) ReadFile(base fi.FileSystem, name string) ([]byte, error) {
	if m == nil {
		return base.ReadFile(name)
	}
	if data, ok := m.lookup(name); ok {
		m.hits.Add(1)
		return data, nil
	}
	m.misses.Add(1)

	for {
		m.mu.Lock()
		if call, ok := m.inflight[name]; ok {
			m.mu.Unlock()
			<-call.done
			if call.err == nil {
				m.hits.Add(1)
			}
			return call.data, call.err
		}
		call := &materializeCall{done: make(chan struct{})}
		m.inflight[name] = call
		m.mu.Unlock()

		data, err := m.produce(base, name)
		call.data, call.err = data, err
		m.mu.Lock()
		delete(m.inflight, name)
		close(call.done)
		m.mu.Unlock()
		return data, err
	}
}

func isJSPSource(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".jsp") || strings.HasSuffix(lower, ".jspx")
}

func (m *SourceMaterializer) lookup(name string) ([]byte, bool) {
	if m.dropMemory {
		// The tree file for a page is generated Java. Reading it back and
		// converting again wraps that Java in out.write(...).
		if isJSPSource(name) {
			return nil, false
		}
		return m.readTree(name)
	}
	if m.overlay == nil {
		return nil, false
	}
	m.overlayMu.Lock()
	defer m.overlayMu.Unlock()
	if _, err := m.overlay.Stat(name); err != nil {
		return nil, false
	}
	data, err := m.overlay.ReadFile(name)
	if err != nil || data == nil {
		return nil, false
	}
	return data, true
}

func (m *SourceMaterializer) produce(base fi.FileSystem, name string) ([]byte, error) {
	// Prefetch and later reads are handed the wrapped filesystem. Reading that
	// wrapper calls back into ReadFile, which is already waiting on this
	// in-flight load, so the unwrap is what keeps the cache from deadlocking.
	base = unwrapMaterializerBase(m, base)
	if data, ok := m.readDisk(name); ok {
		m.diskHits.Add(1)
		m.store(name, data)
		return data, nil
	}
	started := time.Now()
	data, err := base.ReadFile(name)
	m.baseReadNS.Add(time.Since(started).Nanoseconds())
	if err != nil {
		m.errors.Add(1)
		return nil, err
	}
	m.store(name, data)
	m.writeDisk(name, data)
	return data, nil
}

// Put stores an already-transformed source (decompiled class, generated JSP)
// so later reads skip the transform. Empty data is ignored.
func (m *SourceMaterializer) Put(name string, data []byte) {
	if m == nil || len(data) == 0 {
		return
	}
	m.store(name, data)
	if m.treeDir != "" && isJSPSource(name) {
		if err := m.writeTree(name, data); err != nil {
			m.errors.Add(1)
			log.Warnf("[source-materialize] write %s: %v", name, err)
			return
		}
		m.noteTree(name, int64(len(data)))
	}
}

func (m *SourceMaterializer) store(name string, data []byte) {
	if len(data) == 0 {
		return
	}
	started := time.Now()
	defer func() { m.storeNS.Add(time.Since(started).Nanoseconds()) }()
	// Raw JSP must not overwrite the generated .jsp.java. Put writes that file.
	if m.treeDir != "" && !isJSPSource(name) {
		if err := m.writeTree(name, data); err != nil {
			m.errors.Add(1)
			log.Warnf("[source-materialize] write %s: %v", name, err)
			return
		}
	}
	if m.dropMemory {
		if !isJSPSource(name) {
			m.noteTree(name, int64(len(data)))
		}
		return
	}
	if m.overlay == nil {
		return
	}
	m.overlayMu.Lock()
	// VirtualFS.AddFile keeps the first content it sees and silently ignores a
	// later write of the same path. That breaks the JSP flow, which reads the raw
	// page first (caching it) and then Puts the generated Java under the same
	// path. Remove the stale entry so the replacement is what later reads see.
	existed, _ := m.overlay.Stat(name)
	if existed != nil {
		_ = m.overlay.RemoveFileOrDir(name)
		m.loadedB.Add(-existed.Size())
		m.pathCount.Add(-1)
	}
	m.overlay.AddFile(name, string(data))
	m.overlayMu.Unlock()
	m.loadedB.Add(int64(len(data)))
	m.pathCount.Add(1)
}

func (m *SourceMaterializer) diskPath(name string) string {
	if m.diskDir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(name))
	return filepath.Join(m.diskDir, hex.EncodeToString(sum[:])+".src")
}

func (m *SourceMaterializer) readDisk(name string) ([]byte, bool) {
	target := m.diskPath(name)
	if target == "" {
		return nil, false
	}
	data, err := os.ReadFile(target)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

func (m *SourceMaterializer) writeDisk(name string, data []byte) {
	target := m.diskPath(name)
	if target == "" || len(data) == 0 {
		return
	}
	started := time.Now()
	defer func() { m.diskWriteNS.Add(time.Since(started).Nanoseconds()) }()
	if m.maxDisk > 0 && m.diskBytes.Load() > m.maxDisk {
		return
	}
	if _, err := os.Stat(target); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".src-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	m.diskBytes.Add(int64(len(data)))
}

// materializeTreeRel maps a virtual source path onto a .java file. JSP stays
// distinguishable (page.jsp.java) so it cannot collide with page.java.
func materializeTreeRel(name string) (string, error) {
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

func (m *SourceMaterializer) treeFile(name string) (string, error) {
	if m == nil || m.treeDir == "" {
		return "", nil
	}
	rel, err := materializeTreeRel(name)
	if err != nil {
		return "", err
	}
	root := filepath.Clean(m.treeDir)
	full := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	if full != root && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("materialize path %q escapes %s", name, m.treeDir)
	}
	return full, nil
}

func (m *SourceMaterializer) writeTree(name string, data []byte) error {
	target, err := m.treeFile(name)
	if err != nil || target == "" {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".mat-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmpName)
		if werr != nil {
			return werr
		}
		return cerr
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func (m *SourceMaterializer) readTree(name string) ([]byte, bool) {
	target, err := m.treeFile(name)
	if err != nil || target == "" {
		return nil, false
	}
	data, err := os.ReadFile(target)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

func (m *SourceMaterializer) noteTree(name string, n int64) {
	m.overlayMu.Lock()
	defer m.overlayMu.Unlock()
	if m.treeSeen == nil {
		m.treeSeen = map[string]int64{}
	}
	if old, ok := m.treeSeen[name]; ok {
		m.loadedB.Add(n - old)
		m.treeSeen[name] = n
		return
	}
	m.treeSeen[name] = n
	m.pathCount.Add(1)
	m.loadedB.Add(n)
}

// Prefetch materializes every path with a bounded worker pool. Paths already
// materialized are skipped. Read errors are logged at debug level and do not
// abort the prefetch: the compile stage reports them on its own read.
//
// onProgress, when non-nil, is invoked after each completed path. It may be
// called from worker goroutines, so it must be safe for concurrent use.
func (m *SourceMaterializer) Prefetch(
	ctx context.Context,
	base fi.FileSystem,
	paths []string,
	workers int,
	onProgress func(done, total int),
) error {
	if m == nil || base == nil || len(paths) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(paths) {
		workers = len(paths)
	}

	var (
		done     atomic.Int64
		firstErr error
		errOnce  sync.Once
		wg       sync.WaitGroup
	)
	index := make(chan int)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range index {
				if err := ctx.Err(); err != nil {
					errOnce.Do(func() { firstErr = err })
					continue
				}
				path := paths[idx]
				if _, ok := m.lookup(path); ok {
					m.hits.Add(1)
				} else if _, err := m.ReadFile(base, path); err != nil {
					log.Debugf("[source-materialize] read %s failed: %v", path, err)
				} else {
					m.prefetched.Add(1)
				}
				if onProgress != nil {
					onProgress(int(done.Add(1)), len(paths))
				}
			}
		}()
	}
	for i := range paths {
		select {
		case <-ctx.Done():
			errOnce.Do(func() { firstErr = ctx.Err() })
		case index <- i:
			continue
		}
		break
	}
	close(index)
	wg.Wait()
	return firstErr
}

// SourceMaterializerStats is a snapshot of materializer counters.
type SourceMaterializerStats struct {
	Hits       int64
	Misses     int64
	DiskHits   int64
	Errors     int64
	Prefetched int64
	Loaded     int64 // bytes held in the virtual tree
	Paths      int64
	// Worker durations overlap under parallel prefetch; never add these to
	// wall-clock phase durations. BaseRead includes archive I/O + decompilation.
	BaseReadSeconds  float64
	StoreSeconds     float64
	DiskWriteSeconds float64
	DiskWriteBytes   int64
}

// Stats reports the current counters.
func (m *SourceMaterializer) Stats() SourceMaterializerStats {
	if m == nil {
		return SourceMaterializerStats{}
	}
	return SourceMaterializerStats{
		Hits:             m.hits.Load(),
		Misses:           m.misses.Load(),
		DiskHits:         m.diskHits.Load(),
		Errors:           m.errors.Load(),
		Prefetched:       m.prefetched.Load(),
		Loaded:           m.loadedB.Load(),
		Paths:            m.pathCount.Load(),
		BaseReadSeconds:  float64(m.baseReadNS.Load()) / float64(time.Second),
		StoreSeconds:     float64(m.storeNS.Load()) / float64(time.Second),
		DiskWriteSeconds: float64(m.diskWriteNS.Load()) / float64(time.Second),
		DiskWriteBytes:   m.diskBytes.Load(),
	}
}

// materializedSourceFS serves ReadFile from a SourceMaterializer while every
// other filesystem operation (walk, Stat, ReadDir, archive metadata) delegates
// to the wrapped filesystem, so the tree shape seen by the compile stages is
// unchanged.
type materializedSourceFS struct {
	fi.FileSystem
	materializer *SourceMaterializer
}

// WithMaterializedSources wraps base so its reads are served from m. A nil
// materializer returns base unchanged; wrapping twice returns the first wrap.
func WithMaterializedSources(base fi.FileSystem, m *SourceMaterializer) fi.FileSystem {
	if base == nil || m == nil {
		return base
	}
	if _, ok := base.(*materializedSourceFS); ok {
		return base
	}
	return &materializedSourceFS{FileSystem: base, materializer: m}
}

// MaterializerOf returns the materializer attached to fsys, if any.
func MaterializerOf(fsys fi.FileSystem) *SourceMaterializer {
	if w, ok := fsys.(*materializedSourceFS); ok {
		return w.materializer
	}
	return nil
}

func (f *materializedSourceFS) ReadFile(name string) ([]byte, error) {
	return f.materializer.ReadFile(f.FileSystem, name)
}

// unwrapMaterializerBase returns the filesystem underneath a wrap of m.
// produce must read that inner filesystem; the wrapper's ReadFile re-enters
// the materializer and would wait on its own in-flight call.
func unwrapMaterializerBase(m *SourceMaterializer, base fi.FileSystem) fi.FileSystem {
	w, ok := base.(*materializedSourceFS)
	if !ok || w == nil || w.materializer != m || w.FileSystem == nil {
		return base
	}
	return w.FileSystem
}

var _ fi.FileSystem = (*materializedSourceFS)(nil)

// ReleasePreparedSourceCaches releases producer scratch after every prepare
// worker has joined. Reads already materialized in VirtualFS remain available;
// a later uncached read can reconstruct producer caches. No read may overlap
// this phase-boundary operation.
func ReleasePreparedSourceCaches(fsys fi.FileSystem) {
	for fsys != nil {
		switch wrapper := fsys.(type) {
		case *materializedSourceFS:
			fsys = wrapper.FileSystem
		case *UnifiedFS:
			fsys = wrapper.GetFileSystem()
		default:
			if producer, ok := fsys.(interface{ ReleasePreparationCaches() }); ok {
				producer.ReleasePreparationCaches()
			}
			return
		}
	}
}

// PrefetchWorkers returns the worker count for the pre-scan source
// preparation. It uses the full compile concurrency from config (jar, zip,
// class and jsp transforms are independent). YAK_SSA_SOURCE_WORKERS overrides
// that. There is no artificial cap: the caller asked to saturate the
// configured pool.
func PrefetchWorkers(compileConcurrency int) int {
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

// DefaultSourceDiskBudget is the write-through budget for the optional disk
// tier: 8GiB unless overridden by YAK_SSA_SOURCE_CACHE_MAX_BYTES.
func DefaultSourceDiskBudget() int64 {
	if raw := strings.TrimSpace(os.Getenv("YAK_SSA_SOURCE_CACHE_MAX_BYTES")); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > 0 {
			return v
		}
	}
	return 8 << 30
}

// DefaultSourceCacheDir resolves the optional disk tier directory. An empty
// result means the disk tier stays disabled.
//
// YAK_SSA_SOURCE_CACHE_DIR wins; otherwise the user cache dir is used when
// YAK_SSA_SOURCE_CACHE=1/true/disk explicitly asks for it. Default is memory
// only: the decompiled tree is small enough to keep in RAM and a per-task
// YAKIT_HOME would not survive across tasks anyway.
func DefaultSourceCacheDir() string {
	if dir := strings.TrimSpace(os.Getenv("YAK_SSA_SOURCE_CACHE_DIR")); dir != "" {
		return dir
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("YAK_SSA_SOURCE_CACHE"))) {
	case "1", "true", "yes", "on", "disk":
		if dir, err := os.UserCacheDir(); err == nil && dir != "" {
			return filepath.Join(dir, "yaklang", "java-decompile")
		}
	}
	return ""
}

// FormatSourceMaterializerStats renders materializer counters for logs.
func FormatSourceMaterializerStats(stats SourceMaterializerStats) string {
	return fmt.Sprintf(
		"materialized=%d hits=%d misses=%d disk_hits=%d errors=%d prefetched=%d mem=%.1fMB",
		stats.Paths, stats.Hits, stats.Misses, stats.DiskHits, stats.Errors, stats.Prefetched,
		float64(stats.Loaded)/1024/1024,
	)
}

// usableSourceMemory is kept for tests that assert the memory budget policy.
func usableSourceMemory() int64 { return memorybudget.Usable() }
