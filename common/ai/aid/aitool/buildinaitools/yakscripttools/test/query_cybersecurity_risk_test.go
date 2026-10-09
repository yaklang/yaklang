package test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

func TestQueryCybersecurityRiskTool(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("YAKIT_HOME", home)
	t.Setenv("MEMFITAI_HOME", "")
	t.Setenv("SKIP_SYNC_BUILD_IN_AI_TOOL", "true")
	open := func(name string, models ...any) *gorm.DB {
		db, err := gorm.Open("sqlite3", filepath.Join(home, name))
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(models...).Error)
		t.Cleanup(func() { db.Close() })
		return db
	}
	db := open("current.db", &schema.Risk{}, &schema.HTTPFlow{})
	foreign := open("foreign.db", &schema.Risk{}, &schema.HTTPFlow{})
	profile := open(consts.YAK_PROFILE_PLUGIN_DB_NAME, &schema.Project{})
	require.NoError(t, profile.Create(&schema.Project{ProjectName: "risk fixture", DatabasePath: filepath.Join(home, "foreign.db"), Type: "project"}).Error)
	save := func(db *gorm.DB, id uint, riskType, severity, title string, pending bool) {
		record := &schema.Risk{Model: gorm.Model{ID: id}, Hash: fmt.Sprintf("risk-%d", id), Url: "https://example.test/login", Title: title, TitleVerbose: "登录风险", RiskType: riskType, RiskTypeVerbose: "SQL 注入", Severity: severity, Description: "已有证据：\n认证被绕过", Solution: "改用参数化查询", Details: `{"evidence":"fixture proof"}`, Parameter: "username", Payload: "' OR 1=1", RuntimeId: "scan-fixture", WaitingVerified: pending, QuotedRequest: strings.Repeat("packet-must-not-be-loaded", 10000)}
		require.NoError(t, db.Create(record).Error)
	}
	save(db, 1, "sqli", "high", "SQL injection first", false)
	save(db, 2, "sqli", "critical", "SQL injection second", false)
	save(db, 3, "xss", "low", "XSS other", false)
	save(db, 4, "sqli", "high", "Pending risk", true)
	save(foreign, 1, "sqli", "high", "Foreign project risk", false)
	source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/risk/query_cybersecurity_risk.yak")
	require.NoError(t, err)
	metadata := yakscripttools.LoadYakScriptToAiTools("query_cybersecurity_risk", string(source))
	require.NotNil(t, metadata)
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
	require.Len(t, tools, 1)
	require.True(t, aicommon.IsFixedInventoryTool("query_cybersecurity_risk"))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	t.Run("ordinary_script_exports", func(t *testing.T) {
		// Bind only the ordinary global databases. Do not inject a risk/db module,
		// register AI engine hooks, or supply a ToolRuntimeConfig to these engines.
		previous := consts.CaptureProjectDatabaseBinding()
		previousProfile := consts.GetGormProfileDatabase()
		previousProfilePath := consts.GetCurrentProfileDatabasePath()
		consts.BindProjectDatabaseWithReader(db, nil, filepath.Join(home, "current.db"))
		consts.BindProfileDatabase(profile, filepath.Join(home, consts.YAK_PROFILE_PLUGIN_DB_NAME))
		defer func() {
			consts.BindProjectDatabaseWithReader(previous.Database, previous.ReadDatabase, previous.Path)
			consts.BindProfileDatabase(previousProfile, previousProfilePath)
		}()
		flow := &schema.HTTPFlow{Model: gorm.Model{ID: 11}, Hash: "ordinary-script-flow", Url: "https://example.test/login", Method: "POST", StatusCode: 403}
		flow.SetRequest("POST /login HTTP/1.1\r\nHost: example.test\r\n\r\n")
		flow.SetResponse("HTTP/1.1 403 Forbidden\r\n\r\nordinary-script-evidence")
		require.NoError(t, db.Create(flow).Error)
		code := `
page = risk.QueryRiskInDatabase({"type":"sqli", "severity":"high,critical"}, db.keyword("认证被绕过"), db.url("/login"), db.limit(1))~
assert page.Total == 2 && page.HasMore
assert page.Items[0].ID == 2
next = risk.QueryRiskInDatabase({"type":"sqli"}, db.offset(page.NextOffset), db.limit(1))~
assert next.Items[0].ID == 1 && !next.HasMore
assert str.Contains(next.Dump(), "SQL injection first")
bounded = risk.QueryRiskInDatabase({}, db.afterID(1), db.beforeID(3))~
assert bounded.Total == 1 && bounded.Items[0].ID == 2
all = risk.QueryRiskInDatabase(nil)~
assert all.Total == 3
projects = db.ListYakProjects()~
assert len(projects) >= 1
foreignID = ""
for project in projects {
    if project.Name == "risk fixture" { foreignID = project.DatabaseID }
}
assert foreignID != ""
foreignRisk = risk.QueryRiskInDatabase({"ids":[1]}, db.projectID(foreignID))~
assert foreignRisk.Items[0].Title == "Foreign project risk"
history = db.QueryHTTPFlows(db.methods("POST"), db.statusCode("403"), db.packetLimit(128))~
assert history.Total == 1 && history.Items[0].ID == 11
exact = db.QueryHTTPFlowByID(11)~
assert str.Contains(exact.GetResponse(), "ordinary-script-evidence")
`
		require.NoError(t, yaklang.New().Eval(ctx, code), "plain Yak engine must expose the public risk and db libraries")
		_, err := yak.NewScriptEngine(1).ExecuteExWithContext(ctx, code, nil)
		require.NoError(t, err, "ordinary scripts must work without AI runtime bindings")
	})
	execute := func(ctx context.Context, params map[string]any) (*aitool.ToolExecutionResult, error) {
		cfg := aitool.NewToolInvokeConfig()
		aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{ProjectDatabase: db, ProfileDatabase: profile})(cfg)
		return tools[0].ExecuteToolWithCapture(ctx, params, cfg)
	}
	invoke := func(params map[string]any) string {
		t.Helper()
		result, err := execute(ctx, params)
		require.NoError(t, err)
		require.Nil(t, result.Result)
		return result.Stdout
	}
	// Actual script execution: combined filters, stable descending pages and raw text.
	first := invoke(map[string]any{"type": "sqli", "severity": "high,critical", "query": "认证被绕过", "url": "/login", "title": "injection", "runtime_id": "scan-fixture", "limit": 1})
	require.Contains(t, first, "total=2 offset=0 next_offset=1 has_more=true hits=1")
	require.Contains(t, first, "risk_id=2")
	require.NotContains(t, first, "risk_id=1")
	require.Contains(t, first, "已有证据：\n认证被绕过")
	require.Contains(t, first, "fixture proof")
	require.Contains(t, first, "改用参数化查询")
	require.NotContains(t, first, "packet-must-not-be-loaded")
	second := invoke(map[string]any{"type": "sqli", "offset": 1, "limit": 1})
	require.Contains(t, second, "risk_id=1")
	require.Contains(t, second, "has_more=false")
	require.Contains(t, invoke(map[string]any{"risk_id": 1}), "hits=1")
	require.Contains(t, invoke(map[string]any{"after_id": 1, "before_id": 3}), "risk_id=2")
	require.Contains(t, invoke(map[string]any{"query": "no such finding"}), "hits=0")
	require.Contains(t, invoke(map[string]any{"waiting_verified": true}), "Pending risk")
	require.Contains(t, invoke(map[string]any{"query": "参数化查询"}), "hits=3")
	require.Contains(t, invoke(map[string]any{"type": "SQL 注入", "risk_id": 1}), "hits=1")
	// The same risk ID in another registered project must stay isolated.
	projects, err := yaklib.ListYakProjects(yaklib.WithDBHistoryRuntime(ctx, db, profile))
	require.NoError(t, err)
	foreignID := ""
	for _, project := range projects {
		if project.Name == "risk fixture" {
			foreignID = project.DatabaseID
		}
	}
	require.NotEmpty(t, foreignID)
	cross := invoke(map[string]any{"database_id": foreignID, "risk_id": 1})
	require.Contains(t, cross, "Foreign project risk")
	require.NotContains(t, cross, "SQL injection first")
	require.Contains(t, invoke(map[string]any{"risk_id": 1}), "SQL injection first")
	var unchanged schema.Risk
	require.NoError(t, foreign.First(&unchanged, 1).Error)
	require.False(t, unchanged.IsRead)
	require.Equal(t, "Foreign project risk", unchanged.Title)
	// A large UTF-8 field is searched in full but only its bounded excerpt is loaded.
	require.NoError(t, db.Model(&schema.Risk{}).Where("id = ?", 1).UpdateColumn("description", strings.Repeat("证据😀", 10000)+"needle-at-tail").Error)
	large := invoke(map[string]any{"query": "needle-at-tail", "limit": 1})
	require.Contains(t, large, "hits=1")
	require.Contains(t, large, "[truncated:")
	require.True(t, utf8.ValidString(large))
	require.Less(t, len(large), 140000)
	for _, params := range []map[string]any{{"risk_id": -1}, {"limit": 0}, {"limit": 101}, {"offset": -1}, {"database_id": "unknown"}, {"after_id": -1}} {
		_, err := execute(ctx, params)
		require.Error(t, err, "%v", params)
	}
	_, err = yaklib.QueryRiskInDatabase(map[string]any{"unknown": "value"}, yaklib.WithDBHistoryRuntime(ctx, db, profile))
	require.ErrorContains(t, err, "unknown field")
	page, err := yaklib.QueryRiskInDatabase(nil, yaklib.WithDBHistoryRuntime(ctx, db, profile))
	require.NoError(t, err)
	require.Equal(t, 3, page.Total)
	canceled, stop := context.WithCancel(ctx)
	stop()
	_, err = execute(canceled, nil)
	require.Error(t, err)
	t.Logf("query_cybersecurity_risk stdout:\n%s", first)
}
