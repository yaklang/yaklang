package yakit

import (
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestSearchAIForgeBM25_SQLiteFTS5(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&schema.AIForge{}).Error)

	if err := EnsureAIForgeFTS5(db); err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skipf("fts5 not available: %v", err)
		}
		require.NoError(t, err)
	}

	// Insert test forge records
	require.NoError(t, db.Create(&schema.AIForge{
		ForgeName:        "vuln_analyzer",
		ForgeVerboseName: "Vulnerability Analyzer",
		Description:      "Analyze HTTP traffic and identify security vulnerabilities in web applications",
		ToolKeywords:     "vulnerability,security,http,analyze",
		Tags:             "security,analysis",
	}).Error)
	require.NoError(t, db.Create(&schema.AIForge{
		ForgeName:        "report_gen",
		ForgeVerboseName: "Report Generator",
		Description:      "Generate comprehensive security assessment reports in markdown format",
		ToolKeywords:     "report,markdown,assessment",
		Tags:             "report,documentation",
	}).Error)
	require.NoError(t, db.Create(&schema.AIForge{
		ForgeName:        "code_reviewer",
		ForgeVerboseName: "Code Reviewer",
		Description:      "Review source code for security issues and coding best practices",
		ToolKeywords:     "code,review,security,audit",
		Tags:             "code,security",
	}).Error)

	t.Run("BM25 search for vulnerability", func(t *testing.T) {
		got, err := SearchAIForgeBM25(db, &AIForgeSearchFilter{Keywords: "vulnerability"}, 10, 0)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		// vuln_analyzer should rank highest as its name and description contain "vulnerability"
		require.Equal(t, "vuln_analyzer", got[0].ForgeName)
	})

	t.Run("BM25 search for report", func(t *testing.T) {
		got, err := SearchAIForgeBM25(db, &AIForgeSearchFilter{Keywords: "report"}, 10, 0)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		require.Equal(t, "report_gen", got[0].ForgeName)
	})

	t.Run("BM25 search for code review", func(t *testing.T) {
		got, err := SearchAIForgeBM25(db, &AIForgeSearchFilter{Keywords: "code review"}, 10, 0)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		// code_reviewer should match both "code" and "review"
		foundCodeReviewer := false
		for _, f := range got {
			if f.ForgeName == "code_reviewer" {
				foundCodeReviewer = true
				break
			}
		}
		require.True(t, foundCodeReviewer, "expected code_reviewer in results for 'code review'")
	})

	t.Run("BM25 search for security returns multiple", func(t *testing.T) {
		got, err := SearchAIForgeBM25(db, &AIForgeSearchFilter{Keywords: "security"}, 10, 0)
		require.NoError(t, err)
		// "security" appears in vuln_analyzer (description, keywords), code_reviewer (description, keywords, tags),
		// and report_gen (description)
		require.GreaterOrEqual(t, len(got), 2, "expected multiple forges matching 'security'")
	})

	t.Run("short query LIKE fallback", func(t *testing.T) {
		// "vu" is < 3 bytes, should use LIKE fallback instead of FTS5
		got, err := SearchAIForgeBM25(db, &AIForgeSearchFilter{Keywords: "vu"}, 10, 0)
		require.NoError(t, err)
		// LIKE search should still find forges with "vu" substring
		// (e.g. "vuln_analyzer" contains "vu")
		t.Logf("short query 'vu' returned %d results via LIKE fallback", len(got))
	})

	t.Run("empty query returns empty", func(t *testing.T) {
		got, err := SearchAIForgeBM25(db, &AIForgeSearchFilter{Keywords: ""}, 10, 0)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("no match returns empty", func(t *testing.T) {
		got, err := SearchAIForgeBM25(db, &AIForgeSearchFilter{Keywords: "zzzznonexistent"}, 10, 0)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("filter by forge names", func(t *testing.T) {
		got, err := SearchAIForgeBM25(db, &AIForgeSearchFilter{
			ForgeNames: []string{"report_gen"},
			Keywords:   "report",
		}, 10, 0)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		require.Equal(t, "report_gen", got[0].ForgeName)
	})
}

func TestEnsureAIForgeFTS5_Idempotent(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&schema.AIForge{}).Error)

	// Call twice to verify idempotency
	err = EnsureAIForgeFTS5(db)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skipf("fts5 not available: %v", err)
		}
		require.NoError(t, err)
	}

	// Second call should be a no-op
	err = EnsureAIForgeFTS5(db)
	require.NoError(t, err)
}

func TestFilterAIForgeForSearch(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&schema.AIForge{}).Error)

	require.NoError(t, db.Create(&schema.AIForge{
		ForgeName:        "test_forge",
		ForgeVerboseName: "Test Forge",
		Description:      "A test forge for keyword search validation",
		ToolKeywords:     "test,keyword,validation",
		Tags:             "testing",
	}).Error)

	t.Run("keyword LIKE search", func(t *testing.T) {
		var results []*schema.AIForge
		err := FilterAIForgeForSearch(db, &AIForgeSearchFilter{
			Keywords: "keyword",
		}).Find(&results).Error
		require.NoError(t, err)
		require.NotEmpty(t, results)
		require.Equal(t, "test_forge", results[0].ForgeName)
	})

	t.Run("filter by forge names", func(t *testing.T) {
		var results []*schema.AIForge
		err := FilterAIForgeForSearch(db, &AIForgeSearchFilter{
			ForgeNames: []string{"test_forge"},
		}).Find(&results).Error
		require.NoError(t, err)
		require.Len(t, results, 1)
	})

	t.Run("nil filter returns all", func(t *testing.T) {
		var results []*schema.AIForge
		err := FilterAIForgeForSearch(db, nil).Find(&results).Error
		require.NoError(t, err)
		require.NotEmpty(t, results)
	})
}
