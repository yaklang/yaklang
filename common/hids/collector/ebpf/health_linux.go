//go:build hids && linux

package ebpf

import (
	"fmt"
	"strings"
	"sync"
	"time"

	hidscollector "github.com/yaklang/yaklang/common/hids/collector"
)

type ebpfCollectorState struct {
	mu            sync.RWMutex
	name          string
	status        string
	message       string
	updatedAt     time.Time
	received      uint64
	emitted       uint64
	errors        uint64
	decodeErrors  uint64
	readErrors    uint64
	dropped       uint64
	ignored       uint64
	lastEventAt   time.Time
	attachedTrace []string
	skippedTrace  []string
}

func newEBPFCollectorState(name string) ebpfCollectorState {
	return ebpfCollectorState{
		name:      strings.TrimSpace(name),
		status:    "stopped",
		message:   "ebpf collector is stopped",
		updatedAt: time.Now().UTC(),
	}
}

func (s *ebpfCollectorState) setRunning(attached []string, skipped []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status = "running"
	s.message = fmt.Sprintf("ebpf collector is running with %d attached tracepoint(s)", len(attached))
	s.updatedAt = time.Now().UTC()
	s.attachedTrace = cloneStringSlice(attached)
	s.skippedTrace = cloneStringSlice(skipped)
}

func (s *ebpfCollectorState) setStopped() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status = "stopped"
	s.message = "ebpf collector is stopped"
	s.updatedAt = time.Now().UTC()
}

func (s *ebpfCollectorState) observeReceived() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.received++
	s.updatedAt = time.Now().UTC()
}

func (s *ebpfCollectorState) observeDecodeError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.errors++
	s.decodeErrors++
	s.status = "degraded"
	s.message = defaultErrorMessage("ebpf collector decode error", err)
	s.updatedAt = time.Now().UTC()
}

func (s *ebpfCollectorState) observeReadError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.errors++
	s.readErrors++
	s.status = "degraded"
	s.message = defaultErrorMessage("ebpf collector read error", err)
	s.updatedAt = time.Now().UTC()
}

func (s *ebpfCollectorState) observeIgnored() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ignored++
	s.updatedAt = time.Now().UTC()
}

func (s *ebpfCollectorState) observeDropped() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.dropped++
	s.updatedAt = time.Now().UTC()
}

func (s *ebpfCollectorState) observeEmitted(observedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.emitted++
	if !observedAt.IsZero() {
		s.lastEventAt = observedAt.UTC()
	}
	if s.status == "degraded" && s.readErrors == 0 && s.decodeErrors == 0 {
		s.status = "running"
		s.message = fmt.Sprintf("ebpf collector is running with %d attached tracepoint(s)", len(s.attachedTrace))
	}
	s.updatedAt = time.Now().UTC()
}

func (s *ebpfCollectorState) snapshot() hidscollector.HealthSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	kind := ebpfCollectorKind(s.name)
	return hidscollector.HealthSnapshot{
		Name:      kind,
		Backend:   "ebpf",
		Status:    fallbackEBPFString(strings.TrimSpace(s.status), "stopped"),
		Message:   fallbackEBPFString(strings.TrimSpace(s.message), "ebpf collector status unavailable"),
		UpdatedAt: s.updatedAt,
		Detail: map[string]any{
			"stats": map[string]any{
				"received":          s.received,
				"emitted":           s.emitted,
				"errors":            s.errors,
				"dropped":           s.dropped,
				"decode_errors":     s.decodeErrors,
				"read_errors":       s.readErrors,
				"ignored":           s.ignored,
				"last_event_at":     s.lastEventAt,
				"quiet_explanation": ebpfQuietExplanation(kind, s.emitted, s.errors),
			},
			"tracepoints": map[string]any{
				"attached":         cloneStringSlice(s.attachedTrace),
				"optional_skipped": cloneStringSlice(s.skippedTrace),
			},
		},
	}
}

func ebpfCollectorKind(name string) string {
	switch strings.TrimSpace(name) {
	case "ebpf.process":
		return "process"
	case "ebpf.network":
		return "network"
	default:
		return strings.TrimSpace(name)
	}
}

func ebpfQuietExplanation(kind string, emitted uint64, errors uint64) string {
	if emitted > 0 {
		return ""
	}
	if errors > 0 {
		switch kind {
		case "process":
			return "ebpf process collector has not produced process events yet because it has only seen read or decode errors"
		case "network":
			return "ebpf network collector has not produced network lifecycle events yet because it has only seen read or decode errors"
		}
	}
	switch kind {
	case "process":
		return "ebpf process collector is running but has not captured any new process exec or exit event yet"
	case "network":
		return "ebpf network collector is running but has not captured any new connection lifecycle event yet"
	default:
		return "ebpf collector is running but has not captured any event yet"
	}
}

func defaultErrorMessage(prefix string, err error) string {
	if err == nil {
		return prefix
	}
	return fmt.Sprintf("%s: %v", prefix, err)
}

func fallbackEBPFString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
