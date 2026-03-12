package yakit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func setupYakScriptFTSTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.YakScript{}).Error)
	return db
}

func ensureFTSOrSkip(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := EnsureYakScriptForAIFTS5(db); err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skipf("fts5 not available: %v", err)
		}
		require.NoError(t, err)
	}
}

func TestSearchYakScriptForAIBM25_Basic(t *testing.T) {
	db := setupYakScriptFTSTestDB(t)

	require.NoError(t, db.Create(&schema.YakScript{
		ScriptName:  "xss-detect-plugin",
		Type:        "mitm",
		Content:     "// xss plugin",
		Help:        "Detect XSS vulnerabilities",
		EnableForAI: true,
		AIDesc:      "Cross-Site Scripting detection via reflected parameter analysis",
		AIKeywords:  "xss,cross-site scripting,reflected",
	}).Error)
	require.NoError(t, db.Create(&schema.YakScript{
		ScriptName:  "sql-inject-plugin",
		Type:        "mitm",
		Content:     "// sqli plugin",
		Help:        "SQL injection detection",
		EnableForAI: true,
		AIDesc:      "SQL injection detection using UNION and error-based techniques",
		AIKeywords:  "sqli,sql injection,union",
	}).Error)
	require.NoError(t, db.Create(&schema.YakScript{
		ScriptName:  "not-ai-plugin",
		Type:        "mitm",
		Content:     "// not for AI",
		Help:        "Not for AI xss sqli",
		EnableForAI: false,
	}).Error)

	ensureFTSOrSkip(t, db)

	got, err := SearchYakScriptForAIBM25(db, &YakScriptForAIFilter{Keywords: []string{"xss"}}, 10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, got)
	require.Equal(t, "xss-detect-plugin", got[0].ScriptName)

	for _, s := range got {
		require.True(t, s.EnableForAI, "should only return AI-enabled scripts")
		require.NotEqual(t, "not-ai-plugin", s.ScriptName, "non-AI plugin should be excluded")
	}

	got2, err := SearchYakScriptForAIBM25(db, &YakScriptForAIFilter{Keywords: []string{"sql", "injection"}}, 10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, got2)
	found := false
	for _, s := range got2 {
		if s.ScriptName == "sql-inject-plugin" {
			found = true
		}
	}
	require.True(t, found, "should find sql injection plugin")
}

func TestEnsureYakScriptForAIFTS5_Idempotent(t *testing.T) {
	db := setupYakScriptFTSTestDB(t)
	ensureFTSOrSkip(t, db)

	// Second call should be idempotent
	err := EnsureYakScriptForAIFTS5(db)
	require.NoError(t, err)
}

func TestSearchYakScriptForAIBM25_ShortKeywordFallback(t *testing.T) {
	db := setupYakScriptFTSTestDB(t)
	ensureFTSOrSkip(t, db)

	require.NoError(t, db.Create(&schema.YakScript{
		ScriptName:  "ab-plugin",
		Type:        "mitm",
		Content:     "// ab plugin",
		Help:        "AB testing",
		EnableForAI: true,
		AIKeywords:  "ab,test",
	}).Error)

	// "ab" has len < 3, should fall back to LIKE
	got, err := SearchYakScriptForAIBM25(db, &YakScriptForAIFilter{Keywords: []string{"ab"}}, 10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, got, "short keyword should fall back to LIKE and still find results")
	require.Equal(t, "ab-plugin", got[0].ScriptName)
}

func TestSearchYakScriptForAIBM25_OnlyAIEnabled(t *testing.T) {
	db := setupYakScriptFTSTestDB(t)

	// Insert before FTS setup to test migration filtering
	for i := 0; i < 5; i++ {
		require.NoError(t, db.Create(&schema.YakScript{
			ScriptName:  fmt.Sprintf("bulk-plugin-%d", i),
			Type:        "mitm",
			Content:     "// bulk",
			Help:        "bulk security plugin",
			EnableForAI: false,
		}).Error)
	}
	require.NoError(t, db.Create(&schema.YakScript{
		ScriptName:  "ai-security-plugin",
		Type:        "mitm",
		Content:     "// ai security",
		Help:        "AI security testing",
		EnableForAI: true,
		AIDesc:      "Advanced security testing plugin",
		AIKeywords:  "security,testing,advanced",
	}).Error)

	ensureFTSOrSkip(t, db)

	got, err := SearchYakScriptForAIBM25(db, &YakScriptForAIFilter{Keywords: []string{"security"}}, 10, 0)
	require.NoError(t, err)
	require.Len(t, got, 1, "should only find the AI-enabled plugin, not the bulk non-AI ones")
	require.Equal(t, "ai-security-plugin", got[0].ScriptName)
}

func TestSearchYakScriptForAIBM25_TriggerOnUpdate(t *testing.T) {
	db := setupYakScriptFTSTestDB(t)
	ensureFTSOrSkip(t, db)

	// Insert a non-AI plugin
	require.NoError(t, db.Create(&schema.YakScript{
		ScriptName:  "toggle-plugin",
		Type:        "mitm",
		Content:     "// toggle",
		Help:        "Toggle plugin",
		EnableForAI: false,
		AIKeywords:  "toggle,switchable",
	}).Error)

	// Should not be found
	got, err := SearchYakScriptForAIBM25(db, &YakScriptForAIFilter{Keywords: []string{"toggle"}}, 10, 0)
	require.NoError(t, err)
	require.Empty(t, got, "non-AI plugin should not be in FTS results")

	// Toggle to AI-enabled via raw SQL to avoid BeforeSave hook
	require.NoError(t, db.Exec(
		`UPDATE yak_scripts SET enable_for_ai = 1 WHERE script_name = ?`, "toggle-plugin",
	).Error)

	got, err = SearchYakScriptForAIBM25(db, &YakScriptForAIFilter{Keywords: []string{"toggle"}}, 10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, got, "after enabling AI, plugin should appear in FTS results")

	// Toggle back to non-AI
	require.NoError(t, db.Exec(
		`UPDATE yak_scripts SET enable_for_ai = 0 WHERE script_name = ?`, "toggle-plugin",
	).Error)

	got, err = SearchYakScriptForAIBM25(db, &YakScriptForAIFilter{Keywords: []string{"toggle"}}, 10, 0)
	require.NoError(t, err)
	require.Empty(t, got, "after disabling AI, plugin should be removed from FTS results")
}
