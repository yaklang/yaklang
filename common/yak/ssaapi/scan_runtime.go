package ssaapi

import (
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// ScanRuntime is the control point of one scan. It owns the risk collect and
// the registered consumers, and it is the only place that decides whether a
// risk is created or replaces an earlier mode. Consumers apply that decision.
//
// The runtime never writes anything itself. Scanning code submits risks and
// emits results; a database saver, a report writer, or any other consumer
// registers a handler and persists what it needs.
type ScanRuntime struct {
	ID string

	collect  *schema.RiskCollect
	results  []func(*SyntaxFlowResult) error
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
	if r == nil {
		return
	}
	r.collect.SetRuleLevelFunc(fn)
}

// ListenResult registers a consumer of finished syntaxflow results. Results
// are delivered in the order they complete.
func (r *ScanRuntime) ListenResult(fn func(*SyntaxFlowResult) error) {
	if r == nil || fn == nil {
		return
	}
	r.results = append(r.results, fn)
}

// ListenRisk registers a consumer of risk updates. Handlers run in the order
// they were registered.
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

// EmitResult publishes one finished result to every registered consumer.
func (r *ScanRuntime) EmitResult(res *SyntaxFlowResult) error {
	if r == nil || res == nil {
		return nil
	}
	var errs error
	for _, fn := range r.results {
		if fn == nil {
			continue
		}
		if err := fn(res); err != nil {
			errs = utils.JoinErrors(errs, err)
		}
	}
	return errs
}

// SubmitRisk runs the scan's single risk decision and publishes the update.
//
// It reports whether this runtime took the decision. A scan without a runtime
// returns false so the caller keeps its previous behavior.
func (r *ScanRuntime) SubmitRisk(risk *schema.SSARisk) bool {
	if r == nil || risk == nil {
		return false
	}
	item, ok := r.collect.Submit(risk)
	if !ok {
		// A later mode already reported this finding, or a same-mode rule of a
		// higher level covers it. Nothing to publish.
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

// KeepRisk reports whether this risk is the one the scan kept. A report writer
// that receives a rich result uses it to skip a finding a later mode already
// covered, so a late result cannot undo the decision.
func (r *ScanRuntime) KeepRisk(risk *schema.SSARisk) (string, bool) {
	if r == nil || risk == nil {
		return "", false
	}
	key := risk.RiskFeatureHash
	if key == "" {
		return "", true
	}
	cur := r.collect.CurrentRisk(risk)
	if cur == nil {
		return "", false
	}
	if cur == risk || (cur.Hash != "" && cur.Hash == risk.Hash) {
		return "", true
	}
	return "", false
}
