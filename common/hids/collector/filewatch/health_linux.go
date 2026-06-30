//go:build hids && linux

package filewatch

import (
	"fmt"
	"path/filepath"
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
	s.updatedAt = time.Now().UTC()
}

func (s *filewatchCollectorState) observeDirectory(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized := filepath.Clean(path)
	s.dirs[normalized] = struct{}{}
	s.updatedAt = time.Now().UTC()
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
