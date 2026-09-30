package ssaapi

import (
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/schema"
)

// ScanRuntime is the control point of one syntaxflow scan. It owns the risk
// collect and the handlers that apply each decision.
//
// The runtime never writes anything itself. Scanning code submits a risk; the
// collect decides whether that finding is created or replaces an earlier mode,
// and each handler receives that one RiskUpdateItem. Handlers are registered
// before the scan starts submitting, so delivery does not take a lock.
type ScanRuntime struct {
	ID string

	collect *schema.RiskCollect

	risks    []schema.RiskUpdateHandler
	noRiskDB bool
}

// NewScanRuntime creates the runtime of one scan.
func NewScanRuntime() *ScanRuntime {
	return &ScanRuntime{
		ID:      uuid.NewString(),
		collect: schema.NewRiskCollect(),
	}
}

// SetRuleLevelFunc installs the ranking used to filter findings that share a
// feature hash but come from different rules of the same scan mode.
func (r *ScanRuntime) SetRuleLevelFunc(fn func(ruleName string) int) {
	if r == nil || r.collect == nil {
		return
	}
	r.collect.SetRuleLevelFunc(fn)
}

// ListenRisk registers a consumer of risk updates. Handlers run in the order
// they were registered. Register every handler before SubmitRisk.
func (r *ScanRuntime) ListenRisk(handler schema.RiskUpdateHandler) {
	if r == nil || handler == nil {
		return
	}
	r.risks = append(r.risks, handler)
}

// SetNoRiskDB marks this scan as "do not persist risks". Consumers that
// persist rows check this flag; report-only consumers ignore it.
func (r *ScanRuntime) SetNoRiskDB(noSave bool) {
	if r == nil {
		return
	}
	r.noRiskDB = noSave
}

// NoRiskDB reports whether this scan asked not to persist risks.
func (r *ScanRuntime) NoRiskDB() bool {
	if r == nil {
		return false
	}
	return r.noRiskDB
}

// SubmitRisk runs the scan's single risk decision and publishes the update.
//
// It reports whether this runtime took the decision. A nil runtime returns
// false so the caller keeps its previous behavior. A dropped finding (an
// earlier mode, or a lower-level rule of the same mode) returns true and
// publishes nothing.
func (r *ScanRuntime) SubmitRisk(risk *schema.SSARisk) bool {
	if r == nil || risk == nil || r.collect == nil {
		return false
	}
	item, ok := r.collect.Submit(risk)
	if !ok {
		return true
	}
	for _, handler := range r.risks {
		if handler == nil {
			continue
		}
		if err := handler.ApplyRiskUpdate(item); err != nil {
			log.Errorf("apply risk update failed: %v", err)
		}
	}
	return true
}
