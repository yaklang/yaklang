package sfpattern

import (
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// DefaultSourceInFlight is the number of file bodies a streaming source keeps
// at once. Workers block in Read when the cap is reached, so a scan cannot
// pull the whole tree into memory.
const DefaultSourceInFlight = 32

// SourceStore is the file snapshot a source scan reads. Paths are stable and
// carry no content. Read borrows one body; release drops that in-flight slot
// even if the caller still holds a copied string.
type SourceStore interface {
	Paths() []string
	Read(path string) (content string, release func(), err error)
}

type streamState struct {
	fs      fi.FileSystem
	slots   chan struct{}
	reads   atomic.Int64
	current atomic.Int32
	maxSeen atomic.Int32
}

// StreamingSource lists a FileSystem once and reads bodies later, through that
// same interface. The filesystem is wrapped in UnifiedFS so local, virtual and
// zip trees share one path shape.
type StreamingSource struct {
	state *streamState
	paths []string
	limit int
}

// OpenStreamingSource walks fsys without reading file bodies.
// A nil opt lists every file up to 8MiB (the unfiltered loader). A non-nil opt
// uses that opt's size, directory and extension filters.
func OpenStreamingSource(fsys fi.FileSystem, opt *LoadOptions, maxInFlight int) (*StreamingSource, error) {
	if fsys == nil {
		return nil, utils.Error("nil filesystem")
	}
	if _, ok := fsys.(*filesys.UnifiedFS); !ok {
		fsys = filesys.NewUnifiedFS(fsys)
	}
	if maxInFlight < 1 {
		maxInFlight = DefaultSourceInFlight
	}
	paths, err := listSourcePaths(fsys, opt)
	if err != nil {
		return nil, err
	}
	return &StreamingSource{
		state: &streamState{fs: fsys, slots: make(chan struct{}, maxInFlight)},
		paths: paths,
		limit: maxInFlight,
	}, nil
}

// FilterPaths returns a store over the same filesystem and in-flight cap with
// a narrower path list. The cap is shared, so both views cannot exceed it.
func (s *StreamingSource) FilterPaths(keep func(string) bool) *StreamingSource {
	if s == nil {
		return nil
	}
	paths := make([]string, 0, len(s.paths))
	for _, p := range s.paths {
		if keep == nil || keep(p) {
			paths = append(paths, p)
		}
	}
	return &StreamingSource{state: s.state, paths: paths, limit: s.limit}
}

func (s *StreamingSource) Paths() []string {
	if s == nil {
		return nil
	}
	return s.paths
}

func (s *StreamingSource) Limit() int {
	if s == nil || s.limit < 1 {
		return 0
	}
	return s.limit
}

func (s *StreamingSource) ReadCount() int64 {
	if s == nil || s.state == nil {
		return 0
	}
	return s.state.reads.Load()
}

func (s *StreamingSource) MaxInFlight() int {
	if s == nil || s.state == nil {
		return 0
	}
	return int(s.state.maxSeen.Load())
}

func (s *StreamingSource) Read(path string) (string, func(), error) {
	if s == nil || s.state == nil {
		return "", nil, utils.Error("nil source")
	}
	s.state.slots <- struct{}{}
	n := s.state.current.Add(1)
	for {
		old := s.state.maxSeen.Load()
		if n <= old || s.state.maxSeen.CompareAndSwap(old, n) {
			break
		}
	}
	raw, err := s.state.fs.ReadFile(path)
	if err != nil {
		s.state.current.Add(-1)
		<-s.state.slots
		return "", nil, err
	}
	s.state.reads.Add(1)
	var once sync.Once
	release := func() {
		once.Do(func() {
			s.state.current.Add(-1)
			<-s.state.slots
		})
	}
	return string(raw), release, nil
}

func listSourcePaths(fsys fi.FileSystem, opt *LoadOptions) ([]string, error) {
	maxBytes := int64(8 << 20)
	var skip map[string]struct{}
	var ext map[string]struct{}
	if opt != nil {
		maxBytes = opt.MaxFileBytes
		if maxBytes <= 0 {
			maxBytes = 2 << 20
		}
		skip = opt.SkipDirNames
		ext = opt.ExtAllow
	}
	var paths []string
	err := filesys.Recursive(".",
		filesys.WithFileSystem(fsys),
		filesys.WithDirStat(func(name string, info fs.FileInfo) error {
			if opt == nil {
				return nil
			}
			base := path.Base(filepath.ToSlash(name))
			if _, drop := skip[base]; drop {
				return filesys.SkipDir
			}
			return nil
		}),
		filesys.WithFileStat(func(name string, info fs.FileInfo) error {
			if info.IsDir() || info.Size() > maxBytes {
				return nil
			}
			if opt != nil && len(ext) > 0 && !extensionAllowed(name, ext) {
				return nil
			}
			paths = append(paths, filepath.ToSlash(name))
			return nil
		}),
	)
	sort.Strings(paths)
	return paths, err
}

func extensionAllowed(name string, ext map[string]struct{}) bool {
	if _, ok := ext[strings.ToLower(filepath.Ext(name))]; ok {
		return true
	}
	base := strings.ToLower(path.Base(name))
	return strings.HasPrefix(base, ".") || strings.Contains(base, "env") ||
		strings.Contains(base, "dockerfile") || base == "makefile"
}

type mapSource struct {
	files map[string]string
	paths []string
}

// MapSource adapts an already-materialized snapshot. Tests and callers that
// already hold the text use this; the live directory scan does not.
func MapSource(files map[string]string) SourceStore {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	if files == nil {
		files = map[string]string{}
	}
	return &mapSource{files: files, paths: paths}
}

func (m *mapSource) Paths() []string {
	if m == nil {
		return nil
	}
	return m.paths
}

func (m *mapSource) Read(path string) (string, func(), error) {
	if m == nil {
		return "", nil, utils.Error("nil source")
	}
	content, ok := m.files[path]
	if !ok {
		return "", nil, utils.Errorf("source file %q not found", path)
	}
	return content, func() {}, nil
}

func detachString(s string) string {
	if s == "" {
		return ""
	}
	return string([]byte(s))
}
