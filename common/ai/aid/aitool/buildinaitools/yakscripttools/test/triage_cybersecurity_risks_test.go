package test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestTriageCybersecurityRisksTool(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "risk-triage.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.Risk{}).Error)
	t.Cleanup(func() { _ = db.Close() })

	canonical := &schema.Risk{Hash: "canonical", Title: "Canonical", RiskType: "xss", Url: "https://example.test/login"}
	duplicate := &schema.Risk{Hash: "duplicate", Title: "Duplicate", RiskType: "xss", Url: "https://example.test/login"}
	ignored := &schema.Risk{Hash: "ignored", Title: "Ignored", RiskType: "info"}
	require.NoError(t, db.Create(canonical).Error)
	require.NoError(t, db.Create(duplicate).Error)
	require.NoError(t, db.Create(ignored).Error)

	source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/risk/triage_cybersecurity_risks.yak")
	require.NoError(t, err)
	metadata := yakscripttools.LoadYakScriptToAiTools("triage_cybersecurity_risks", string(source))
	require.NotNil(t, metadata)
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
	require.Len(t, tools, 1)
	require.True(t, aicommon.IsFixedInventoryTool("triage_cybersecurity_risks"))
	toolSchema := tools[0].ToJSONSchemaString()
	require.NotContains(t, toolSchema, `"delete"`)
	require.NotContains(t, toolSchema, `"hard_delete"`)

	execute := func(params map[string]any) (*aitool.ToolExecutionResult, error) {
		cfg := aitool.NewToolInvokeConfig()
		aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{ProjectDatabase: db})(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return tools[0].ExecuteToolWithCapture(ctx, params, cfg)
	}

	result, err := execute(map[string]any{
		"risk_ids":     "2",
		"disposition":  schema.RiskDispositionDuplicate,
		"duplicate_of": canonical.ID,
		"reason":       "same target, type, and input parameter",
	})
	require.NoError(t, err)
	require.Contains(t, result.Stdout, "disposition=duplicate")
	require.Contains(t, result.Stdout, "updated=1")

	var stored schema.Risk
	require.NoError(t, db.First(&stored, duplicate.ID).Error)
	require.Equal(t, schema.RiskDispositionDuplicate, stored.Disposition)
	require.Equal(t, canonical.ID, stored.DuplicateOfRiskID)
	require.Equal(t, "same target, type, and input parameter", stored.TagReason)

	result, err = execute(map[string]any{
		"risk_ids":    "3",
		"disposition": schema.RiskDispositionIgnored,
		"reason":      "not security relevant",
	})
	require.NoError(t, err)
	require.Contains(t, result.Stdout, "updated=1")

	result, err = execute(map[string]any{
		"risk_ids":    "3",
		"disposition": schema.RiskDispositionActive,
	})
	require.NoError(t, err)
	require.Contains(t, result.Stdout, "disposition=active")

	var count int64
	require.NoError(t, db.Model(&schema.Risk{}).Count(&count).Error)
	require.EqualValues(t, 3, count, "triage must never hard-delete risks")

	_, err = execute(map[string]any{
		"risk_ids":     "2",
		"disposition":  schema.RiskDispositionDuplicate,
		"duplicate_of": duplicate.ID,
		"reason":       "self reference",
	})
	require.ErrorContains(t, err, "cannot be one of the risks")

	_, err = execute(map[string]any{
		"risk_ids":    "2",
		"disposition": schema.RiskDispositionFalsePositive,
	})
	require.ErrorContains(t, err, "reason is required")

	scopedCfg := aitool.NewToolInvokeConfig()
	aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{
		ProjectDatabase: db,
		RiskSelectionScope: &aitool.RiskSelectionScope{
			AllowedIDs: []int64{int64(canonical.ID)},
		},
	})(scopedCfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = tools[0].ExecuteToolWithCapture(ctx, map[string]any{
		"risk_ids":    "3",
		"disposition": schema.RiskDispositionIgnored,
		"reason":      "outside attachment",
	}, scopedCfg)
	require.ErrorContains(t, err, "outside the attached risk selection")
}

func TestRiskQueryAndTriageSchemasAreFixedInventory(t *testing.T) {
	require.True(t, aicommon.IsFixedInventoryTool("query_cybersecurity_risk"))
	require.True(t, aicommon.IsFixedInventoryTool("triage_cybersecurity_risks"))
}

func TestRiskDispositionHelpers(t *testing.T) {
	require.True(t, schema.IsValidRiskDisposition(""))
	require.True(t, schema.IsValidRiskDisposition(schema.RiskDispositionDuplicate))
	require.False(t, schema.IsValidRiskDisposition("deleted"))
	require.Equal(t, schema.RiskDispositionActive, schema.NormalizeRiskDisposition(""))

	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "risk-disposition-helper.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.Risk{}).Error)
	t.Cleanup(func() { _ = db.Close() })

	risk := &schema.Risk{Hash: "helper"}
	require.NoError(t, db.Create(risk).Error)
	_, err = yakit.UpdateRiskDispositions(db, []int64{0}, schema.RiskDispositionIgnored, 0, "reason")
	require.ErrorContains(t, err, "must be positive")

	_, err = yaklib.TriageRiskInDatabase(
		[]int64{int64(risk.ID)},
		schema.RiskDispositionIgnored,
		0,
		"outside scope",
		yaklib.WithDBHistoryRuntime(context.Background(), db, nil),
		yaklib.WithDBHistoryRiskSelectionScope([]int64{int64(risk.ID) + 1}),
	)
	require.ErrorContains(t, err, "outside the attached risk selection")
}
