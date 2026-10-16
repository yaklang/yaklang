package schema

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func collectRisk(feature, mode, rule string, id uint) *SSARisk {
	risk := &SSARisk{
		Hash:            feature + "-" + mode + "-" + rule,
		RiskFeatureHash: feature,
		ScanMode:        mode,
		FromRule:        rule,
		CodeSourceUrl:   "file:///main.go",
	}
	risk.ID = id
	return risk
}

func TestRiskCollect_LaterModeCoversEarlier(t *testing.T) {
	c := NewRiskCollect()
	source := collectRisk("feature-1", string(SFR_MODE_SOURCE), "source-rule", 11)
	item, ok := c.Submit(source)
	require.True(t, ok)
	require.Equal(t, RiskUpdateCreate, item.Mode)
	ssa := collectRisk("feature-1", string(SFR_MODE_SSA), "ssa-rule", 0)
	item, ok = c.Submit(ssa)
	require.True(t, ok)
	require.Equal(t, RiskUpdateUpdate, item.Mode)
	require.Equal(t, source.Hash, item.OldHash)
	require.Zero(t, ssa.ID, "a cover does not copy the stored row id")
	require.Equal(t, ssa, c.Current(ssa))
}

func TestRiskCollect_CoverOfUnsavedRiskIsUpdate(t *testing.T) {
	c := NewRiskCollect()
	source := collectRisk("feature-unsaved", string(SFR_MODE_SOURCE), "source-rule", 0)
	if _, ok := c.Submit(source); !ok {
		t.Fatal("the first finding must be kept")
	}
	ssa := collectRisk("feature-unsaved", string(SFR_MODE_SSA), "ssa-rule", 0)
	item, ok := c.Submit(ssa)
	require.True(t, ok)
	require.Equal(t, RiskUpdateUpdate, item.Mode)
	require.Equal(t, source.Hash, item.OldHash)
	require.Zero(t, ssa.ID)
}

func TestRiskCollect_EarlierModeIsDropped(t *testing.T) {
	c := NewRiskCollect()
	ssa := collectRisk("feature-1", string(SFR_MODE_SSA), "ssa-rule", 7)
	if _, ok := c.Submit(ssa); !ok {
		t.Fatal("the first finding must be kept")
	}
	source := collectRisk("feature-1", string(SFR_MODE_SOURCE), "source-rule", 0)
	_, ok := c.Submit(source)
	require.False(t, ok, "an earlier mode may not replace a later one")
	require.Equal(t, ssa, c.Current(ssa))
}

func TestRiskCollect_RuleLevelFiltersSameMode(t *testing.T) {
	c := NewRiskCollect()
	c.SetRuleLevelFunc(func(ruleName string) int {
		switch ruleName {
		case "high":
			return 2
		case "low":
			return 1
		default:
			return 0
		}
	})
	high := collectRisk("feature-1", string(SFR_MODE_SSA), "high", 3)
	if _, ok := c.Submit(high); !ok {
		t.Fatal("the first finding must be kept")
	}
	low := collectRisk("feature-1", string(SFR_MODE_SSA), "low", 0)
	_, ok := c.Submit(low)
	require.False(t, ok, "a lower-level rule of the same mode is filtered out")
	require.Equal(t, high, c.Current(high))

	higher := collectRisk("feature-1", string(SFR_MODE_SSA), "high", 0)
	_, ok = c.Submit(higher)
	require.True(t, ok, "the same level keeps the later finding")
}

func TestRiskCollect_SeverityFiltersSameModeByDefault(t *testing.T) {
	c := NewRiskCollect()
	low := collectRisk("feature-1", string(SFR_MODE_SSA), "low-rule", 4)
	low.Severity = SFR_SEVERITY_LOW
	if _, ok := c.Submit(low); !ok {
		t.Fatal("the first finding must be kept")
	}
	// A weaker report of the same finding may not replace the stronger one.
	weaker := collectRisk("feature-1", string(SFR_MODE_SSA), "info-rule", 0)
	weaker.Severity = SFR_SEVERITY_INFO
	_, ok := c.Submit(weaker)
	require.False(t, ok)
	require.Equal(t, low, c.Current(low))

	// A stronger report replaces it.
	stronger := collectRisk("feature-1", string(SFR_MODE_SSA), "high-rule", 0)
	stronger.Severity = SFR_SEVERITY_HIGH
	item, ok := c.Submit(stronger)
	require.True(t, ok)
	require.Equal(t, RiskUpdateUpdate, item.Mode)
	require.Equal(t, low.Hash, item.OldHash)
}

func TestRiskCollect_FeatureHashIsTheCoverKey(t *testing.T) {
	c := NewRiskCollect()
	source := collectRisk("same-feature", string(SFR_MODE_SOURCE), "source-rule", 9)
	_, ok := c.Submit(source)
	require.True(t, ok)
	ssa := collectRisk("same-feature", string(SFR_MODE_SSA), "ssa-rule", 0)
	item, ok := c.Submit(ssa)
	require.True(t, ok)
	require.Equal(t, source.Hash, item.OldHash)
	require.Equal(t, RiskUpdateUpdate, item.Mode)
	other := collectRisk("other-feature", string(SFR_MODE_SSA), "ssa-rule", 0)
	item, ok = c.Submit(other)
	require.True(t, ok)
	require.Empty(t, item.OldHash, "a different feature hash is a different finding")
}

func TestRiskCollect_EmptyFeatureHashIsAlwaysKept(t *testing.T) {
	c := NewRiskCollect()
	first := collectRisk("", string(SFR_MODE_SOURCE), "rule", 0)
	item, ok := c.Submit(first)
	require.True(t, ok)
	require.Equal(t, RiskUpdateCreate, item.Mode)
	second := collectRisk("", string(SFR_MODE_SOURCE), "rule", 0)
	_, ok = c.Submit(second)
	require.True(t, ok, "a finding without a feature hash never covers another")
}

// positionRange builds the position json the producer writes into CodeRange.
func positionRange(url string, line, column int64) string {
	return `{"url":"` + url + `","start_line":` + strconv.FormatInt(line, 10) +
		`,"start_column":` + strconv.FormatInt(column, 10) +
		`,"end_line":` + strconv.FormatInt(line, 10) +
		`,"end_column":` + strconv.FormatInt(column+1, 10) + `}`
}

// TestRiskCollect_SiblingStatementsStaySeparate proves the cover key carries the
// source position as well as the feature hash: two statements that share a
// value text are two findings, not one covered finding.
func TestRiskCollect_SiblingStatementsStaySeparate(t *testing.T) {
	c := NewRiskCollect()
	first := collectRisk("sibling-feature", string(SFR_MODE_SSA), "rule", 0)
	first.CodeRange = positionRange("/prog/src/a.go", 14, 30)
	if _, ok := c.Submit(first); !ok {
		t.Fatal("the first finding must be kept")
	}
	sibling := collectRisk("sibling-feature", string(SFR_MODE_SSA), "rule", 0)
	sibling.Hash = "sibling-hash"
	sibling.CodeRange = positionRange("/prog/src/a.go", 15, 20)
	item, ok := c.Submit(sibling)
	require.True(t, ok)
	require.Equal(t, RiskUpdateCreate, item.Mode,
		"a different statement is a new finding even when the feature hash matches")
	require.Empty(t, item.OldHash)
}

// TestRiskCollect_ProgramNameDoesNotSplitTheCoverKey proves the same source
// compiled under another program name keeps one cover key, so an overlay scan
// still covers the row of its base program.
func TestRiskCollect_ProgramNameDoesNotSplitTheCoverKey(t *testing.T) {
	c := NewRiskCollect()
	base := collectRisk("program-feature", string(SFR_MODE_SOURCE), "rule", 0)
	base.ProgramName = "prog-1"
	base.CodeSourceUrl = "/prog-1/src/a.go"
	base.CodeRange = positionRange("/prog-1/src/a.go", 14, 30)
	if _, ok := c.Submit(base); !ok {
		t.Fatal("the first finding must be kept")
	}
	overlay := collectRisk("program-feature", string(SFR_MODE_SSA), "rule", 0)
	overlay.ProgramName = "prog-2"
	overlay.CodeSourceUrl = "/prog-2/src/a.go"
	overlay.CodeRange = positionRange("/prog-2/src/a.go", 14, 30)
	item, ok := c.Submit(overlay)
	require.True(t, ok)
	require.Equal(t, RiskUpdateUpdate, item.Mode)
	require.Equal(t, base.Hash, item.OldHash)
}
