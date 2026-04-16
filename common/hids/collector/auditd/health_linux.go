//go:build hids && linux

package auditd

import (
	"fmt"
	"strings"
	"sync"
	"time"

	hidscollector "github.com/yaklang/yaklang/common/hids/collector"
)

type auditCollectorState struct {
	mu            sync.RWMutex
	status        string
	message       string
	updatedAt     time.Time
	received      uint64
	emitted       uint64
	filtered      uint64
	normalizeErr  uint64
	loss          uint64
	lastEventAt   time.Time
	rulesTotal    int
	rulesAdded    int
	rulesExisting int
	rulesSkipped  int
	families      map[string]uint64
	reasons       map[string]uint64
}

func newAuditCollectorState() auditCollectorState {
	return auditCollectorState{
		status:    "stopped",
		message:   "audit collector is stopped",
		updatedAt: time.Now().UTC(),
		families:  make(map[string]uint64),
		reasons:   make(map[string]uint64),
	}
}

func (s *auditCollectorState) setStatus(status string, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status = strings.TrimSpace(status)
	s.message = strings.TrimSpace(message)
	s.updatedAt = time.Now().UTC()
}

func (s *auditCollectorState) setRuleInstallResult(result auditRuleInstallResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.rulesTotal = result.total
	s.rulesAdded = result.added
	s.rulesExisting = result.existing
	s.rulesSkipped = result.skipped
	s.updatedAt = time.Now().UTC()
}

func (s *auditCollectorState) observeReceived() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.received++
	s.updatedAt = time.Now().UTC()
}

func (s *auditCollectorState) observeOutcome(outcome auditObservationOutcome, observedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !observedAt.IsZero() {
		s.lastEventAt = observedAt.UTC()
	}
	if family := strings.TrimSpace(outcome.family); family != "" {
		s.families[family]++
	}
	if outcome.normalizeError {
		s.normalizeErr++
	}
	if outcome.keep {
		s.emitted++
		if strings.TrimSpace(s.status) == "" || s.status == "stopped" {
			s.status = "running"
			s.message = "audit collector is running"
		}
		s.updatedAt = time.Now().UTC()
		return
	}

	s.filtered++
	if reason := strings.TrimSpace(outcome.filterReason); reason != "" {
		s.reasons[reason]++
	}
	s.updatedAt = time.Now().UTC()
}

func (s *auditCollectorState) observeLoss(observedAt time.Time, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.loss++
	s.families["loss"]++
	if !observedAt.IsZero() {
		s.lastEventAt = observedAt.UTC()
	}
	if normalized := strings.TrimSpace(reason); normalized != "" {
		s.reasons[normalized]++
	}
	s.updatedAt = time.Now().UTC()
}

func (s *auditCollectorState) snapshot(journalAvailable bool) hidscollector.HealthSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	families := make(map[string]uint64, len(s.families))
	for key, value := range s.families {
		families[key] = value
	}
	reasons := make(map[string]uint64, len(s.reasons))
	for key, value := range s.reasons {
		reasons[key] = value
	}

	detail := map[string]any{
		"journal_available": journalAvailable,
		"managed_rules": map[string]any{
			"key_prefix": hidsAuditRuleKeyPrefix,
			"total":      s.rulesTotal,
			"added":      s.rulesAdded,
			"existing":   s.rulesExisting,
			"skipped":    s.rulesSkipped,
		},
		"stats": map[string]any{
			"received":          s.received,
			"emitted":           s.emitted,
			"filtered":          s.filtered,
			"normalize_errors":  s.normalizeErr,
			"loss":              s.loss,
			"families":          families,
			"filter_reasons":    reasons,
			"last_event_at":     s.lastEventAt,
			"quiet_explanation": auditQuietExplanation(s.received, s.emitted, s.filtered, s.normalizeErr),
		},
	}

	status := strings.TrimSpace(s.status)
	if status == "" {
		status = "stopped"
	}
	message := strings.TrimSpace(s.message)
	if message == "" {
		message = fmt.Sprintf("audit collector status: %s", status)
	}

	return hidscollector.HealthSnapshot{
		Name:      "audit",
		Backend:   "auditd",
		Status:    status,
		Message:   message,
		UpdatedAt: s.updatedAt,
		Detail:    detail,
	}
}

func auditQuietExplanation(received uint64, emitted uint64, filtered uint64, normalizeErr uint64) string {
	switch {
	case received == 0:
		return "audit collector is running but has not received any coalesced audit event yet"
	case emitted == 0 && filtered > 0:
		return "audit collector is receiving events, but current traffic is being filtered as non-HIDS noise"
	case normalizeErr > 0 && emitted == 0:
		return "audit collector has encountered normalization errors before producing HIDS-facing audit events"
	default:
		return ""
	}
}
