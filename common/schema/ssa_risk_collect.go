package schema

import (
	"strconv"
	"strings"
	"sync"
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
	mu   sync.Mutex
	data map[string]*SSARisk
	// ruleLevel ranks rules of the same scan mode. When two rules of equal
	// mode report one feature hash, the higher level wins; equal levels keep
	// the later finding. Nil means every rule has level 0.
	ruleLevel func(ruleName string) int
}

func NewRiskCollect() *RiskCollect {
	return &RiskCollect{data: map[string]*SSARisk{}}
}

// SetRuleLevelFunc installs the rule-level ranking used to filter findings
// that share one feature hash but come from different rules of the same mode.
func (c *RiskCollect) SetRuleLevelFunc(fn func(ruleName string) int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.ruleLevel = fn
	c.mu.Unlock()
}

// Current returns the risk kept for this feature hash.
func (c *RiskCollect) Current(feature string) *SSARisk {
	return c.currentByKey(strings.TrimSpace(feature))
}

// CurrentRisk returns the risk kept for the same key Submit would use, which
// includes the reported position so two sibling findings inside one function
// stay distinct.
func (c *RiskCollect) CurrentRisk(risk *SSARisk) *SSARisk {
	if c == nil || risk == nil {
		return nil
	}
	feature := strings.TrimSpace(risk.RiskFeatureHash)
	if feature == "" {
		return nil
	}
	return c.currentByKey(riskCollectKey(feature, risk))
}

func (c *RiskCollect) currentByKey(key string) *SSARisk {
	if c == nil {
		return nil
	}
	if key == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		return nil
	}
	return c.data[key]
}

// riskCollectKey combines the feature hash with the reported position. Two
// sibling statements can share a feature hash (same function and value text),
// and only the statement itself may be covered by a later mode.
func riskCollectKey(feature string, risk *SSARisk) string {
	key := feature
	if risk == nil {
		return key
	}
	if rangeInfo := strings.TrimSpace(risk.CodeRange); rangeInfo != "" {
		return key + "\x00" + rangeInfo
	}
	if risk.Line > 0 {
		return key + "\x00line:" + strconv.FormatInt(risk.Line, 10)
	}
	return key
}

// Submit records in and returns the update the caller must publish.
//
// When the feature hash is new the returned item has OldID 0. When an earlier
// risk is replaced the item carries that row's id and hash. When the incoming
// risk loses to the stored one no update is returned (ok=false), which is how
// an earlier mode is dropped and how a lower-level rule is filtered out.
func (c *RiskCollect) Submit(in *SSARisk) (RiskUpdateItem, bool) {
	item := RiskUpdateItem{Risk: in}
	if c == nil || in == nil {
		return item, false
	}
	key := strings.TrimSpace(in.RiskFeatureHash)
	if key == "" {
		return item, true
	}
	key = riskCollectKey(key, in)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = map[string]*SSARisk{}
	}
	old, exist := c.data[key]
	if !exist {
		c.data[key] = in
		return item, true
	}
	if !c.betterLocked(in, old) {
		return item, false
	}
	if old != nil {
		item.OldID = old.ID
		item.OldHash = old.Hash
	}
	if in.ID == 0 {
		in.ID = item.OldID
	}
	c.data[key] = in
	return item, true
}

// betterLocked reports whether incoming should replace existing.
func (c *RiskCollect) betterLocked(incoming, existing *SSARisk) bool {
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
	}
	// Same mode, same level: the later finding is the more complete one.
	return true
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
