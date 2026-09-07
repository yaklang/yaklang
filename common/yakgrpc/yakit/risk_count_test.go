package yakit

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

func newRiskCountTestDB(t *testing.T) *gorm.DB {
	db, err := consts.CreateProjectDatabase(filepath.Join(t.TempDir(), "risk-count.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.Risk{}).Error)
	return db
}

func TestNormalizeRiskSeverity(t *testing.T) {
	require.Equal(t, "critical", NormalizeRiskSeverity("PANIC"))
	require.Equal(t, "critical", NormalizeRiskSeverity(" fatal "))
	require.Equal(t, "high", NormalizeRiskSeverity("high"))
	require.Equal(t, "warning", NormalizeRiskSeverity("Middle"))
	require.Equal(t, "warning", NormalizeRiskSeverity("medium"))
	require.Equal(t, "warning", NormalizeRiskSeverity("warn"))
	require.Equal(t, "low", NormalizeRiskSeverity("default"))
	require.Equal(t, "info", NormalizeRiskSeverity(""))
	require.Equal(t, "info", NormalizeRiskSeverity("fingerprint"))
	require.Equal(t, "other", NormalizeRiskSeverity("unknown-level"))
}

func TestCountRiskByRuntimeIds_MultiRuntime(t *testing.T) {
	db := newRiskCountTestDB(t)

	risks := []*schema.Risk{
		{Hash: "h1", Url: "http://a/1", RuntimeId: "rt-1", RiskType: "sqli", Severity: "high", Title: "t1"},
		{Hash: "h2", Url: "http://a/2", RuntimeId: "rt-1", RiskType: "xss", Severity: "critical", Title: "t2"},
		{Hash: "h3", Url: "http://b/1", RuntimeId: "rt-2", RiskType: "info", Severity: "low", Title: "t3"},
		// middle 属于历史写法，应当归入 warning 统计
		{Hash: "h4", Url: "http://b/2", RuntimeId: "rt-2", RiskType: "info", Severity: "middle", Title: "t4"},
		// 空等级应当归入 info
		{Hash: "h5", Url: "http://b/3", RuntimeId: "rt-2", RiskType: "info", Title: "t5"},
		// 不属于任何被统计的 runtime
		{Hash: "h6", Url: "http://c/1", RuntimeId: "rt-3", RiskType: "sqli", Severity: "critical", Title: "t6"},
		// 完全未知的等级归入 other，但依然计入 Total
		{Hash: "h7", Url: "http://c/2", RuntimeId: "rt-2", RiskType: "sqli", Severity: "super-critical", Title: "t7"},
	}
	for _, r := range risks {
		require.NoError(t, CreateOrUpdateRisk(db, r.Hash, r))
	}

	stat, err := CountRiskByRuntimeIds(db, "rt-1", "rt-2", "", "rt-1")
	require.NoError(t, err)
	require.NotNil(t, stat)

	require.EqualValues(t, 1, stat.Critical)
	require.EqualValues(t, 1, stat.High)
	require.EqualValues(t, 1, stat.Warning)
	require.EqualValues(t, 1, stat.Low)  // h3
	require.EqualValues(t, 1, stat.Info) // h5，schema.BeforeSave 会将空等级补齐为 info
	require.EqualValues(t, 1, stat.Other)
	require.EqualValues(t, 6, stat.Total) // h6 属于 rt-3，不计入
	require.EqualValues(t, stat.Critical+stat.High+stat.Warning+stat.Low+stat.Info+stat.Other, stat.Total)

	// 单个 runtime 也支持
	stat2, err := CountRiskByRuntimeIds(db, "rt-1")
	require.NoError(t, err)
	require.EqualValues(t, 2, stat2.Total)
	require.EqualValues(t, 1, stat2.High)
	require.EqualValues(t, 1, stat2.Critical)

	// 空 runtimeId 应当报错而不是全表统计
	empty, err := CountRiskByRuntimeIds(db, "", "  ")
	require.Error(t, err)
	require.Nil(t, empty)

	// 兼容旧的单 runtime 总数接口
	total, err := CountRiskByRuntimeId(db, "rt-2")
	require.NoError(t, err)
	require.Equal(t, 4, total)
}
