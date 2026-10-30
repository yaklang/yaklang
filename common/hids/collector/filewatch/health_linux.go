//go:build hids && linux

package filewatch

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	hidscollector "github.com/yaklang/yaklang/common/hids/collector"
)

type filewatchCollectorState struct {
	mu          sync.RWMutex
	status      string
	message     string
	updatedAt   time.Time
	roots       []string
	dirs        map[string]struct{}
	parents     map[string]int
	anchors     map[string]struct{}
	received    uint64
	emitted     uint64
	errors      uint64
	dropped     uint64
	lastEventAt time.Time
}

func newFilewatchCollectorState(roots []string) filewatchCollectorState {
	return filewatchCollectorState{
		status:    "stopped",
		message:   "filewatch collector is stopped",
		updatedAt: time.Now().UTC(),
		roots:     cloneFilewatchStrings(roots),
		dirs:      make(map[string]struct{}),
		parents:   make(map[string]int),
		anchors:   filewatchRootAnchors(roots),
	}
}

func (s *filewatchCollectorState) setRunning() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status = "running"
	s.message = fmt.Sprintf("filewatch collector is running on %d root path(s)", len(s.roots))
	s.updatedAt = time.Now().UTC()
}

func (s *filewatchCollectorState) setStopped() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status = "stopped"
	s.message = "filewatch collector is stopped"
	s.dirs = make(map[string]struct{})
	s.parents = make(map[string]int)
	s.updatedAt = time.Now().UTC()
}

func (s *filewatchCollectorState) observeDirectory(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized := filepath.Clean(path)
	if _, exists := s.dirs[normalized]; exists {
		return
	}
	s.dirs[normalized] = struct{}{}
	s.adjustParents(normalized, 1)
	s.updatedAt = time.Now().UTC()
}

func (s *filewatchCollectorState) hasDirectory(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.dirs[filepath.Clean(path)]
	return ok
}

func (s *filewatchCollectorState) hasDirectoryTree(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	path = filepath.Clean(path)
	_, exists := s.dirs[path]
	return exists || s.parents[path] > 0
}

func (s *filewatchCollectorState) adjustParents(path string, delta int) {
	for {
		if _, root := s.anchors[path]; root {
			return
		}
		parent := filepath.Dir(path)
		if parent == path {
			return
		}
		if s.parents[parent]+delta <= 0 {
			delete(s.parents, parent)
		} else {
			s.parents[parent] += delta
		}
		path = parent
	}
}

func filewatchRootAnchors(roots []string) map[string]struct{} {
	configured := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		configured[filepath.Clean(root)] = struct{}{}
	}
	anchors := make(map[string]struct{}, len(configured))
	for root := range configured {
		covered := false
		for path := root; ; {
			parent := filepath.Dir(path)
			if parent == path {
				break
			}
			if _, exists := configured[parent]; exists {
				covered = true
				break
			}
			path = parent
		}
		if !covered {
			anchors[root] = struct{}{}
		}
	}
	return anchors
}

func (s *filewatchCollectorState) forgetDirectoryTree(root string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	root = filepath.Clean(root)
	prefix := strings.TrimSuffix(root, string(filepath.Separator)) + string(filepath.Separator)
	var removed []string
	for path := range s.dirs {
		if path == root || strings.HasPrefix(path, prefix) {
			removed = append(removed, path)
			delete(s.dirs, path)
			s.adjustParents(path, -1)
		}
	}
	if len(removed) > 0 {
		s.updatedAt = time.Now().UTC()
	}
	return removed
}

func (s *filewatchCollectorState) observeReceived() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.received++
	s.updatedAt = time.Now().UTC()
}

func (s *filewatchCollectorState) observeEmitted(observedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.emitted++
	if !observedAt.IsZero() {
		s.lastEventAt = observedAt.UTC()
	}
	s.updatedAt = time.Now().UTC()
}

func (s *filewatchCollectorState) observeDropped() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.dropped++
	s.updatedAt = time.Now().UTC()
}

func (s *filewatchCollectorState) observeError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.errors++
	if s.status == "stopped" {
		return
	}
	s.status = "degraded"
	if err != nil {
		s.message = fmt.Sprintf("filewatch collector error: %v", err)
	} else {
		s.message = "filewatch collector reported a watcher error"
	}
	s.updatedAt = time.Now().UTC()
}

func (s *filewatchCollectorState) snapshot() hidscollector.HealthSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return hidscollector.HealthSnapshot{
		Name:      "file",
		Backend:   "filewatch",
		Status:    s.status,
		Message:   s.message,
		UpdatedAt: s.updatedAt,
		Detail: map[string]any{
			"stats": map[string]any{
				"received":          s.received,
				"emitted":           s.emitted,
				"errors":            s.errors,
				"dropped":           s.dropped,
				"last_event_at":     s.lastEventAt,
				"quiet_explanation": filewatchQuietExplanation(s.emitted, s.errors),
			},
			"watch": map[string]any{
				"roots":       cloneFilewatchStrings(s.roots),
				"directories": len(s.dirs),
			},
		},
	}
}

func filewatchQuietExplanation(emitted uint64, errors uint64) string {
	if emitted > 0 {
		return ""
	}
	if errors > 0 {
		return "filewatch collector has not produced file change events yet because watcher errors occurred first"
	}
	return "filewatch collector is running but has not captured any new file change event yet"
}

func cloneFilewatchStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
