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

// FileCache is a read-through content cache that sits in front of a
// FileSystem.
//
// It owns one policy only: cached bytes stay in memory while they fit the
// memory limit, and each entry that does not fit is written to a private
// directory under the overflow parent instead. Memory and disk keep the same
// tree of names, so a caller reads an entry back without knowing where it
// lives.
//
// ReadFile(base, name) returns the cached bytes on a hit. On a miss it calls
// the loader exactly once, and concurrent readers of the same name wait for
// that one call instead of repeating the work. The default loader reads the
// base filesystem; a caller can replace it to produce content instead of
// copying it.
//
// Put stores bytes for a name and replaces an existing entry, so a caller that
// transforms content can publish the result under the same name. An optional
// publisher observes every stored entry and can mirror it elsewhere.
//
// The cache is not a filesystem on its own. WrapWithFileCache attaches it to a
// base filesystem: ReadFile is served from the cache, and every other
// operation delegates to the base, so walks, metadata and paths keep the
// original tree shape.
//
// All exported operations are safe for concurrent use. Close drops every entry
// and removes the overflow directory.
type FileCache struct {
	loader  func(fi.FileSystem, string) ([]byte, error)
	publish func(string, []byte)

	memoryLimit    int64
	overflowParent string

	mu          sync.Mutex
	entries     map[string]*cacheEntry
	memBytes    int64
	diskBytes   int64
	inflight    map[string]*cacheCall
	overflowDir string
	closed      bool

	hits           atomic.Int64
	misses         atomic.Int64
	errors         atomic.Int64
	prefetched     atomic.Int64
	baseReadNS     atomic.Int64
	storeNS        atomic.Int64
	diskWriteNS    atomic.Int64
	diskWriteCount atomic.Int64
}

type cacheEntry struct {
	size int64
	data []byte // nil once the entry lives on disk
	file string // absolute overflow path, empty while data is in memory
}

type cacheCall struct {
	done chan struct{}
	data []byte
	err  error
}

// FileCacheOption configures a FileCache.
type FileCacheOption func(*FileCache)

// WithFileCacheMemoryLimit bounds the bytes retained in memory. Once the
// retained set would exceed limit, new entries are written to the private
// overflow directory instead. limit <= 0 keeps every entry in memory.
func WithFileCacheMemoryLimit(limit int64) FileCacheOption {
	return func(s *FileCache) {
		if limit > 0 {
			s.memoryLimit = limit
		}
	}
}

// WithFileCacheOverflowDir places the private overflow directory under parent
// (default: os.TempDir()). The directory is created on the first overflow.
func WithFileCacheOverflowDir(parent string) FileCacheOption {
	return func(s *FileCache) {
		if parent != "" {
			s.overflowParent = parent
		}
	}
}

// WithFileCachePublisher observes every stored entry (a load or a Put). Callers
// use it to mirror prepared content into a real directory. It runs after the
// cache insert, without the cache lock held.
func WithFileCachePublisher(publish func(string, []byte)) FileCacheOption {
	return func(s *FileCache) {
		if publish != nil {
			s.publish = publish
		}
	}
}

// WithFileCacheLoader replaces the default loader (base.ReadFile). It runs once
// per path miss and must not call back into the same FileCache for that path.
func WithFileCacheLoader(load func(fi.FileSystem, string) ([]byte, error)) FileCacheOption {
	return func(s *FileCache) {
		if load != nil {
			s.loader = load
		}
	}
}

// NewFileCache creates an empty cache.
func NewFileCache(opts ...FileCacheOption) *FileCache {
	s := &FileCache{
		entries:  make(map[string]*cacheEntry),
		inflight: make(map[string]*cacheCall),
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
func (s *FileCache) ReadFile(base fi.FileSystem, name string) ([]byte, error) {
	if s == nil {
		return base.ReadFile(name)
	}
	// The compile passes the wrapper that serves reads from this cache. Using
	// it as the loader base would re-enter ReadFile and wait on its own
	// in-flight call, so always load from the inner filesystem.
	base = UnwrapFileCache(base, s)
	missed := false
	var call *cacheCall
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
		call = &cacheCall{done: make(chan struct{})}
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
func (s *FileCache) Put(name string, data []byte) {
	if s == nil || len(data) == 0 {
		return
	}
	s.store(name, data)
}

func (s *FileCache) lookup(name string) ([]byte, bool, error) {
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
		// A missing overflow file degrades to a reload, not a hard failure.
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

func (s *FileCache) store(name string, data []byte) {
	started := time.Now()
	defer func() { s.storeNS.Add(time.Since(started).Nanoseconds()) }()

	overflow := false
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
		overflow = true
	}
	s.mu.Unlock()

	if overflow {
		if file, err := s.overflowFile(name, data); err == nil {
			s.mu.Lock()
			if s.closed {
				s.mu.Unlock()
				_ = os.Remove(file)
				return
			}
			s.entries[name] = &cacheEntry{size: size, file: file}
			s.diskBytes += size
			s.mu.Unlock()
			s.notify(name, data)
			return
		}
		// A failed overflow keeps the bytes in memory: correctness over budget.
		log.Warnf("[file-cache] overflow %s failed, keeping it in memory", name)
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.entries[name] = &cacheEntry{size: size, data: data}
	s.memBytes += size
	s.mu.Unlock()
	if oldFile != "" {
		_ = os.Remove(oldFile)
	}
	s.notify(name, data)
}

func (s *FileCache) overflowFile(name string, data []byte) (string, error) {
	s.mu.Lock()
	dir := s.overflowDir
	if dir == "" && !s.closed {
		parent := s.overflowParent
		if parent == "" {
			parent = os.TempDir()
		}
		created, err := os.MkdirTemp(parent, "yaklang-cache-*")
		if err != nil {
			s.mu.Unlock()
			return "", err
		}
		s.overflowDir = created
		dir = created
	}
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return "", fmt.Errorf("file cache is closed")
	}

	sum := sha256.Sum256([]byte(name))
	target := filepath.Join(dir, hex.EncodeToString(sum[:])+".src")
	started := time.Now()
	defer func() { s.diskWriteNS.Add(time.Since(started).Nanoseconds()) }()
	tmp, err := os.CreateTemp(dir, ".cache-*")
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
	s.diskWriteCount.Add(1)
	return target, nil
}

func (s *FileCache) notify(name string, data []byte) {
	if s.publish != nil {
		s.publish(name, data)
	}
}

// Prefetch loads every path through the cache with workers. Individual read
// failures are logged and counted; only a cancelled context stops the walk.
func (s *FileCache) Prefetch(ctx context.Context, base fi.FileSystem, paths []string, workers int, onProgress func(done, total int)) error {
	if s == nil || base == nil || len(paths) == 0 {
		return nil
	}
	base = UnwrapFileCache(base, s)
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
					log.Warnf("[file-cache] read %s failed: %v", path, err)
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
func (s *FileCache) WalkEntries(fn func(name string, size int64) error) error {
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

// FileCacheStats is a snapshot of cache counters. Worker durations overlap under
// parallel prefetch and are never wall-clock phase durations.
type FileCacheStats struct {
	Hits       int64
	Misses     int64
	Errors     int64
	Prefetched int64
	Paths      int64
	MemoryHold int64 // bytes retained in memory
	DiskHold   int64 // bytes retained in the private overflow directory
	BaseRead   float64
	Store      float64
	DiskWrite  float64 // time spent writing overflow files
	DiskWrites int64
}

// Stats reports the current counters.
func (s *FileCache) Stats() FileCacheStats {
	if s == nil {
		return FileCacheStats{}
	}
	s.mu.Lock()
	paths := int64(len(s.entries))
	mem, disk := s.memBytes, s.diskBytes
	s.mu.Unlock()
	return FileCacheStats{
		Hits:       s.hits.Load(),
		Misses:     s.misses.Load(),
		Errors:     s.errors.Load(),
		Prefetched: s.prefetched.Load(),
		Paths:      paths,
		MemoryHold: mem,
		DiskHold:   disk,
		BaseRead:   float64(s.baseReadNS.Load()) / float64(time.Second),
		Store:      float64(s.storeNS.Load()) / float64(time.Second),
		DiskWrite:  float64(s.diskWriteNS.Load()) / float64(time.Second),
		DiskWrites: s.diskWriteCount.Load(),
	}
}

// Close drops every entry and removes the private overflow directory.
func (s *FileCache) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	dir := s.overflowDir
	s.overflowDir = ""
	s.entries = make(map[string]*cacheEntry)
	s.memBytes, s.diskBytes = 0, 0
	s.mu.Unlock()
	if dir == "" {
		return nil
	}
	return os.RemoveAll(dir)
}

// cachedSourceFS serves ReadFile from a FileCache and delegates everything else
// to the wrapped filesystem. Re-entering the wrapper from the loader would
// wait on the in-flight call, so the loader always receives the inner FS.
type cachedSourceFS struct {
	fi.FileSystem
	cache *FileCache
}

// WrapWithFileCache serves reads from the cache while preserving the base tree.
// A nil cache or base returns base unchanged.
func WrapWithFileCache(base fi.FileSystem, cache *FileCache) fi.FileSystem {
	if base == nil || cache == nil {
		return base
	}
	if w, ok := base.(*cachedSourceFS); ok && w.cache == cache {
		return base
	}
	return &cachedSourceFS{FileSystem: base, cache: cache}
}

// UnwrapFileCache returns the filesystem underneath a cache wrapper.
func UnwrapFileCache(base fi.FileSystem, cache *FileCache) fi.FileSystem {
	if w, ok := base.(*cachedSourceFS); ok && w.cache == cache && w.FileSystem != nil {
		return w.FileSystem
	}
	return base
}

// FileCacheOf returns the cache attached to fsys, if any.
func FileCacheOf(fsys fi.FileSystem) *FileCache {
	if w, ok := fsys.(*cachedSourceFS); ok {
		return w.cache
	}
	return nil
}

func (f *cachedSourceFS) ReadFile(name string) ([]byte, error) {
	return f.cache.ReadFile(f.FileSystem, name)
}

var _ fi.FileSystem = (*cachedSourceFS)(nil)

// ReleasePreparationCaches calls ReleasePreparationCaches on every producer
// hidden under the wrappers. Cached sources stay readable; only scratch DFA
// state is dropped. No read may overlap this phase-boundary call.
func ReleasePreparationCaches(fsys fi.FileSystem) {
	for fsys != nil {
		switch wrapper := fsys.(type) {
		case *cachedSourceFS:
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

// FormatFileCacheStats renders cache counters for logs.
func FormatFileCacheStats(stats FileCacheStats) string {
	return fmt.Sprintf(
		"cached=%d hits=%d misses=%d prefetched=%d errors=%d memory=%.1fMB disk=%.1fMB",
		stats.Paths, stats.Hits, stats.Misses, stats.Prefetched, stats.Errors,
		float64(stats.MemoryHold)/1024/1024, float64(stats.DiskHold)/1024/1024,
	)
}
