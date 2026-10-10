package schema

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// RiskUpdateItem is one decision the scan reached about a single finding.
//
// OldID 0 means "create a new row". Any other OldID means "rewrite that row",
// which happens when a later scan mode covers a finding an earlier mode
// already reported. OldHash is the previous row hash so consumers holding a
// copy of the old data (for example a JSON report) can drop it.
type RiskUpdateItem struct {
	Risk    *SSARisk
	OldID   uint
	OldHash string
}

// RiskUpdateHandler consumes the decisions of one scan. A scan never writes a
// row itself; a database saver, a report writer, or any other consumer
// registers here and applies the update its own way.
type RiskUpdateHandler interface {
	ApplyRiskUpdate(RiskUpdateItem) error
}

// RiskUpdateHandlerFunc adapts a function to RiskUpdateHandler.
type RiskUpdateHandlerFunc func(RiskUpdateItem) error

func (f RiskUpdateHandlerFunc) ApplyRiskUpdate(item RiskUpdateItem) error {
	if f == nil {
		return nil
	}
	return f(item)
}

// RiskCollect keeps one scan's findings keyed by feature hash so a later scan
// mode can replace an earlier row for the same finding. The stored risk is
// kept only to recognize and rewrite that row.
//
// The collector never writes anything. Submit reports the decision to the
// caller so the caller can hand a RiskUpdateItem to its handlers.
type RiskCollect struct {
	data *utils.SafeMap[*SSARisk]
	// ruleLevel optionally ranks rules of the same scan mode. When two rules
	// of equal mode report one finding, the higher level wins; equal levels
	// keep the later finding. Nil falls back to ranking by severity so a
	// stronger rule covers a weaker one. It is installed before Submit runs.
	ruleLevel func(ruleName string) int
}

func NewRiskCollect() *RiskCollect {
	return &RiskCollect{data: utils.NewSafeMap[*SSARisk]()}
}

// SetRuleLevelFunc installs the rule-level ranking used to filter findings
// that share one feature hash but come from different rules of the same mode.
func (c *RiskCollect) SetRuleLevelFunc(fn func(ruleName string) int) {
	if c == nil {
		return
	}
	c.ruleLevel = fn
}

// Current returns the risk kept for this feature hash.
func (c *RiskCollect) Current(feature string) *SSARisk {
	if c == nil || c.data == nil {
		return nil
	}
	feature = strings.TrimSpace(feature)
	if feature == "" {
		return nil
	}
	risk, ok := c.data.Get(feature)
	if !ok {
		return nil
	}
	return risk
}

// Submit records in and returns the update the caller must publish.
//
// The key is only the feature hash. When it is new the returned item has
// OldID 0. When an earlier risk is replaced the item carries that row's id
// and hash. When the incoming risk loses to the stored one no update is
// returned (ok=false), which is how an earlier mode is dropped and how a
// lower-level rule is filtered out. An empty feature hash is always kept and
// is not stored, so it never covers another finding.
func (c *RiskCollect) Submit(in *SSARisk) (RiskUpdateItem, bool) {
	item := RiskUpdateItem{Risk: in}
	if c == nil || in == nil {
		return item, false
	}
	key := strings.TrimSpace(in.RiskFeatureHash)
	if key == "" {
		return item, true
	}
	if c.data == nil {
		c.data = utils.NewSafeMap[*SSARisk]()
	}
	accepted := false
	c.data.Update(key, func(old *SSARisk, loaded bool) (*SSARisk, bool) {
		if !loaded {
			accepted = true
			return in, true
		}
		if !c.better(in, old) {
			return old, false
		}
		if old != nil {
			item.OldID = old.ID
			item.OldHash = old.Hash
		}
		if in.ID == 0 {
			in.ID = item.OldID
		}
		accepted = true
		return in, true
	})
	return item, accepted
}

// better reports whether incoming should replace existing.
func (c *RiskCollect) better(incoming, existing *SSARisk) bool {
	if incoming == nil {
		return false
	}
	if existing == nil {
		return true
	}
	inMode := ValidRuleMode(incoming.ScanMode)
	oldMode := ValidRuleMode(existing.ScanMode)
	if inMode != oldMode {
		return laterScanMode(inMode, oldMode)
	}
	if c.ruleLevel != nil {
		inLevel := c.ruleLevel(incoming.FromRule)
		oldLevel := c.ruleLevel(existing.FromRule)
		if inLevel != oldLevel {
			return inLevel > oldLevel
		}
		// Same mode and same configured level: the later finding wins.
		return true
	}
	// Different rules of one mode may report the same finding; keep the more
	// severe report so a stronger rule covers a weaker one.
	inSeverity := severityRank(incoming.Severity)
	oldSeverity := severityRank(existing.Severity)
	if inSeverity != oldSeverity {
		return inSeverity > oldSeverity
	}
	// Same mode, same level: the later finding is the more complete one.
	return true
}

// severityRank orders severities so a stronger rule can cover a weaker one.
func severityRank(severity SyntaxFlowSeverity) int {
	switch ValidSeverityType(severity) {
	case SFR_SEVERITY_CRITICAL:
		return 5
	case SFR_SEVERITY_HIGH:
		return 4
	case SFR_SEVERITY_WARNING:
		return 3
	case SFR_SEVERITY_LOW:
		return 2
	default:
		return 1
	}
}

// laterScanMode reports whether a risk of mode incoming may replace a risk of
// mode existing. Same mode keeps the later one; source yields to struct and
// ssa, struct yields to ssa.
func laterScanMode(incoming, existing SyntaxFlowRuleModeType) bool {
	switch existing {
	case SFR_MODE_SOURCE:
		return true
	case SFR_MODE_STRUCT:
		return incoming != SFR_MODE_SOURCE
	default:
		return incoming == SFR_MODE_SSA
	}
}
