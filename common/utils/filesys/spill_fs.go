package filesys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// SpillFS is a language-agnostic content cache in front of a FileSystem.
//
// It owns exactly one policy: retained source bytes stay in memory until a
// budget, then overflow into a private temporary directory. Callers decide
// what a miss means through the loader; the default loader reads the base
// filesystem.
//
// ReadFile(base, name) returns the cached source on a hit and runs the loader
// exactly once on a miss (concurrent callers for the same path wait for that
// load). Put replaces an entry, so a caller that transforms a file in place
// (page -> generated Java under the same path) can publish the result without
// a second filesystem.
//
// The cache is not a filesystem by itself: WrapWithSpillFS attaches it to a
// base filesystem, and every operation except ReadFile delegates to that base,
// so walks and metadata keep the original tree shape.
type SpillFS struct {
	loader  func(fi.FileSystem, string) ([]byte, error)
	publish func(string, []byte)

	memoryLimit int64
	tempParent  string

	mu        sync.Mutex
	entries   map[string]*spillEntry
	memBytes  int64
	diskBytes int64
	inflight  map[string]*spillCall
	dir       string
	closed    bool

	hits        atomic.Int64
	misses      atomic.Int64
	errors      atomic.Int64
	prefetched  atomic.Int64
	baseReadNS  atomic.Int64
	storeNS     atomic.Int64
	spillNS     atomic.Int64
	spillWrites atomic.Int64
}

type spillEntry struct {
	size int64
	data []byte // nil once the entry lives on disk
	file string // absolute spill path, empty while data is in memory
}

type spillCall struct {
	done chan struct{}
	data []byte
	err  error
}

// SpillFSOption configures a SpillFS.
type SpillFSOption func(*SpillFS)

// WithSpillMemoryLimit bounds the bytes retained in memory. Once the retained
// set would exceed limit, new entries are written to the private temp
// directory instead. limit <= 0 keeps every entry in memory.
func WithSpillMemoryLimit(limit int64) SpillFSOption {
	return func(s *SpillFS) {
		if limit > 0 {
			s.memoryLimit = limit
		}
	}
}

// WithSpillTempParent places the private spill directory under parent
// (default: os.TempDir()). The directory is created on the first spill.
func WithSpillTempParent(parent string) SpillFSOption {
	return func(s *SpillFS) {
		if parent != "" {
			s.tempParent = parent
		}
	}
}

// WithSpillPublisher observes every stored entry (a load or a Put). Callers
// use it to mirror prepared content into a real directory. It runs after the
// cache insert, without the cache lock held.
func WithSpillPublisher(publish func(string, []byte)) SpillFSOption {
	return func(s *SpillFS) {
		if publish != nil {
			s.publish = publish
		}
	}
}

// WithSpillLoader replaces the default loader (base.ReadFile). It runs once
// per path miss and must not call back into the same SpillFS for that path.
func WithSpillLoader(load func(fi.FileSystem, string) ([]byte, error)) SpillFSOption {
	return func(s *SpillFS) {
		if load != nil {
			s.loader = load
		}
	}
}

// NewSpillFS creates an empty cache.
func NewSpillFS(opts ...SpillFSOption) *SpillFS {
	s := &SpillFS{
		entries:  make(map[string]*spillEntry),
		inflight: make(map[string]*spillCall),
	}
	s.loader = func(base fi.FileSystem, name string) ([]byte, error) {
		return base.ReadFile(name)
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

// ReadFile returns the cached bytes for name, loading it once on a miss.
func (s *SpillFS) ReadFile(base fi.FileSystem, name string) ([]byte, error) {
	if s == nil {
		return base.ReadFile(name)
	}
	// The compile passes the wrapper that serves reads from this cache. Using
	// it as the loader base would re-enter ReadFile and wait on its own
	// in-flight call, so always load from the inner filesystem.
	base = UnwrapSpillFS(base, s)
	missed := false
	var call *spillCall
	for {
		if data, ok, err := s.lookup(name); ok {
			if err != nil {
				s.errors.Add(1)
				return nil, err
			}
			s.hits.Add(1)
			return data, nil
		}
		if !missed {
			s.misses.Add(1)
			missed = true
		}

		s.mu.Lock()
		if call, ok := s.inflight[name]; ok {
			s.mu.Unlock()
			<-call.done
			if call.err == nil {
				s.hits.Add(1)
			}
			return call.data, call.err
		}
		// A producer may have published the entry between the unlocked lookup
		// above and taking this lock. Publishing registers the in-flight call
		// first, so a present entry with no in-flight call is complete; retry
		// the lookup instead of producing the same source a second time.
		if _, ok := s.entries[name]; ok {
			s.mu.Unlock()
			continue
		}
		call = &spillCall{done: make(chan struct{})}
		s.inflight[name] = call
		s.mu.Unlock()
		break
	}

	started := time.Now()
	data, err := s.loader(base, name)
	s.baseReadNS.Add(time.Since(started).Nanoseconds())
	if err != nil {
		s.errors.Add(1)
	} else {
		s.store(name, data)
	}
	call.data, call.err = data, err
	s.mu.Lock()
	delete(s.inflight, name)
	close(call.done)
	s.mu.Unlock()
	return data, err
}

// Put publishes transformed content for name, replacing any existing entry.
// Empty data is ignored.
func (s *SpillFS) Put(name string, data []byte) {
	if s == nil || len(data) == 0 {
		return
	}
	s.store(name, data)
}

func (s *SpillFS) lookup(name string) ([]byte, bool, error) {
	s.mu.Lock()
	entry, ok := s.entries[name]
	s.mu.Unlock()
	if !ok {
		return nil, false, nil
	}
	if entry.data != nil {
		return entry.data, true, nil
	}
	if entry.file == "" {
		return nil, false, nil
	}
	data, err := os.ReadFile(entry.file)
	if err != nil {
		// A missing spill file degrades to a reload, not a hard failure.
		s.mu.Lock()
		if current, exists := s.entries[name]; exists && current == entry {
			delete(s.entries, name)
			s.diskBytes -= entry.size
		}
		s.mu.Unlock()
		return nil, false, err
	}
	return data, true, nil
}

func (s *SpillFS) store(name string, data []byte) {
	started := time.Now()
	defer func() { s.storeNS.Add(time.Since(started).Nanoseconds()) }()

	spill := false
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	oldFile := ""
	if old, ok := s.entries[name]; ok {
		if old.data != nil {
			s.memBytes -= old.size
		} else {
			s.diskBytes -= old.size
			oldFile = old.file
		}
		delete(s.entries, name)
	}
	size := int64(len(data))
	if s.memoryLimit > 0 && s.memBytes+size > s.memoryLimit {
		spill = true
	}
	s.mu.Unlock()

	if spill {
		if file, err := s.spillFile(name, data); err == nil {
			s.mu.Lock()
			if s.closed {
				s.mu.Unlock()
				_ = os.Remove(file)
				return
			}
			s.entries[name] = &spillEntry{size: size, file: file}
			s.diskBytes += size
			s.mu.Unlock()
			s.notify(name, data)
			return
		}
		// A failed spill keeps the bytes in memory: correctness over budget.
		log.Warnf("[spill-fs] spill %s failed, keeping it in memory", name)
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.entries[name] = &spillEntry{size: size, data: data}
	s.memBytes += size
	s.mu.Unlock()
	if oldFile != "" {
		_ = os.Remove(oldFile)
	}
	s.notify(name, data)
}

func (s *SpillFS) spillFile(name string, data []byte) (string, error) {
	s.mu.Lock()
	dir := s.dir
	if dir == "" && !s.closed {
		parent := s.tempParent
		if parent == "" {
			parent = os.TempDir()
		}
		created, err := os.MkdirTemp(parent, "yaklang-spill-*")
		if err != nil {
			s.mu.Unlock()
			return "", err
		}
		s.dir = created
		dir = created
	}
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return "", fmt.Errorf("spill filesystem is closed")
	}

	sum := sha256.Sum256([]byte(name))
	target := filepath.Join(dir, hex.EncodeToString(sum[:])+".src")
	started := time.Now()
	defer func() { s.spillNS.Add(time.Since(started).Nanoseconds()) }()
	tmp, err := os.CreateTemp(dir, ".spill-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmpName)
		if werr != nil {
			return "", werr
		}
		return "", cerr
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	s.spillWrites.Add(1)
	return target, nil
}

func (s *SpillFS) notify(name string, data []byte) {
	if s.publish != nil {
		s.publish(name, data)
	}
}

// Prefetch loads every path through the cache with workers. Individual read
// failures are logged and counted; only a cancelled context stops the walk.
func (s *SpillFS) Prefetch(ctx context.Context, base fi.FileSystem, paths []string, workers int, onProgress func(done, total int)) error {
	if s == nil || base == nil || len(paths) == 0 {
		return nil
	}
	base = UnwrapSpillFS(base, s)
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
				if _, ok, _ := s.lookup(path); ok {
					s.hits.Add(1)
				} else if _, err := s.ReadFile(base, path); err != nil {
					log.Warnf("[spill-fs] read %s failed: %v", path, err)
				} else {
					s.prefetched.Add(1)
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

// WalkEntries visits every cached path with its size. It does not hold the
// cache lock while fn runs, so fn may call ReadFile/Put.
func (s *SpillFS) WalkEntries(fn func(name string, size int64) error) error {
	if s == nil || fn == nil {
		return nil
	}
	type entry struct {
		name string
		size int64
	}
	s.mu.Lock()
	entries := make([]entry, 0, len(s.entries))
	for name, e := range s.entries {
		entries = append(entries, entry{name: name, size: e.size})
	}
	s.mu.Unlock()
	for _, e := range entries {
		if err := fn(e.name, e.size); err != nil {
			return err
		}
	}
	return nil
}

// SpillFSStats is a snapshot of cache counters. Worker durations overlap under
// parallel prefetch and are never wall-clock phase durations.
type SpillFSStats struct {
	Hits        int64
	Misses      int64
	Errors      int64
	Prefetched  int64
	Paths       int64
	MemoryHold  int64 // bytes retained in memory
	DiskHold    int64 // bytes retained in the private spill directory
	BaseRead    float64
	Store       float64
	Spill       float64
	SpillWrites int64
}

// Stats reports the current counters.
func (s *SpillFS) Stats() SpillFSStats {
	if s == nil {
		return SpillFSStats{}
	}
	s.mu.Lock()
	paths := int64(len(s.entries))
	mem, disk := s.memBytes, s.diskBytes
	s.mu.Unlock()
	return SpillFSStats{
		Hits:        s.hits.Load(),
		Misses:      s.misses.Load(),
		Errors:      s.errors.Load(),
		Prefetched:  s.prefetched.Load(),
		Paths:       paths,
		MemoryHold:  mem,
		DiskHold:    disk,
		BaseRead:    float64(s.baseReadNS.Load()) / float64(time.Second),
		Store:       float64(s.storeNS.Load()) / float64(time.Second),
		Spill:       float64(s.spillNS.Load()) / float64(time.Second),
		SpillWrites: s.spillWrites.Load(),
	}
}

// Close drops every entry and removes the private spill directory.
func (s *SpillFS) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	dir := s.dir
	s.dir = ""
	s.entries = make(map[string]*spillEntry)
	s.memBytes, s.diskBytes = 0, 0
	s.mu.Unlock()
	if dir == "" {
		return nil
	}
	return os.RemoveAll(dir)
}

// spillSourceFS serves ReadFile from a SpillFS and delegates everything else
// to the wrapped filesystem. Re-entering the wrapper from the loader would
// wait on the in-flight call, so the loader always receives the inner FS.
type spillSourceFS struct {
	fi.FileSystem
	spill *SpillFS
}

// WrapWithSpillFS serves reads from spill while preserving the base tree.
// A nil spill or base returns base unchanged.
func WrapWithSpillFS(base fi.FileSystem, spill *SpillFS) fi.FileSystem {
	if base == nil || spill == nil {
		return base
	}
	if w, ok := base.(*spillSourceFS); ok && w.spill == spill {
		return base
	}
	return &spillSourceFS{FileSystem: base, spill: spill}
}

// UnwrapSpillFS returns the filesystem underneath a wrapper of spill.
func UnwrapSpillFS(base fi.FileSystem, spill *SpillFS) fi.FileSystem {
	if w, ok := base.(*spillSourceFS); ok && w.spill == spill && w.FileSystem != nil {
		return w.FileSystem
	}
	return base
}

// SpillOf returns the cache attached to fsys, if any.
func SpillOf(fsys fi.FileSystem) *SpillFS {
	if w, ok := fsys.(*spillSourceFS); ok {
		return w.spill
	}
	return nil
}

func (f *spillSourceFS) ReadFile(name string) ([]byte, error) {
	return f.spill.ReadFile(f.FileSystem, name)
}

var _ fi.FileSystem = (*spillSourceFS)(nil)

// ReleasePreparationCaches calls ReleasePreparationCaches on every producer
// hidden under the wrappers. Cached sources stay readable; only scratch DFA
// state is dropped. No read may overlap this phase-boundary call.
func ReleasePreparationCaches(fsys fi.FileSystem) {
	for fsys != nil {
		switch wrapper := fsys.(type) {
		case *spillSourceFS:
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

// FormatSpillFSStats renders cache counters for logs.
func FormatSpillFSStats(stats SpillFSStats) string {
	return fmt.Sprintf(
		"cached=%d hits=%d misses=%d prefetched=%d errors=%d memory=%.1fMB disk=%.1fMB",
		stats.Paths, stats.Hits, stats.Misses, stats.Prefetched, stats.Errors,
		float64(stats.MemoryHold)/1024/1024, float64(stats.DiskHold)/1024/1024,
	)
}
