package schema

import (
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
	if _, ok := c.Submit(source); !ok {
		t.Fatal("the first finding must be kept")
	}
	ssa := collectRisk("feature-1", string(SFR_MODE_SSA), "ssa-rule", 0)
	item, ok := c.Submit(ssa)
	require.True(t, ok)
	require.Equal(t, uint(11), item.OldID)
	require.Equal(t, source.Hash, item.OldHash)
	require.Equal(t, ssa, c.Current("feature-1"))
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
	require.Equal(t, ssa, c.Current("feature-1"))
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
	require.Equal(t, high, c.Current("feature-1"))

	higher := collectRisk("feature-1", string(SFR_MODE_SSA), "high", 0)
	_, ok = c.Submit(higher)
	require.True(t, ok, "the same level keeps the later finding")
}

func TestRiskCollect_EmptyFeatureHashIsAlwaysKept(t *testing.T) {
	c := NewRiskCollect()
	first := collectRisk("", string(SFR_MODE_SOURCE), "rule", 0)
	item, ok := c.Submit(first)
	require.True(t, ok)
	require.Zero(t, item.OldID)
	second := collectRisk("", string(SFR_MODE_SOURCE), "rule", 0)
	_, ok = c.Submit(second)
	require.True(t, ok, "a finding without a feature hash never covers another")
}
