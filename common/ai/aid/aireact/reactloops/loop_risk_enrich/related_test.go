package loop_risk_enrich

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func riskTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.Risk{}, &schema.AISession{}, &schema.AIAgentRuntime{}, &schema.HTTPFlow{}, &schema.Port{}).Error)
	return db
}

func TestSessionRuntimeAcrossToolsFindsTargetOnly(t *testing.T) {
	db := riskTestDB(t)
	r := &schema.Risk{Hash: "session-risk", Host: "example.test", AISessionID: "sess", RuntimeId: "tool-a", Details: strconv.Quote(`{"target_input":"https://example.test/login"}`)}
	require.NoError(t, db.Create(r).Error)
	require.NoError(t, db.Create(&schema.AISession{SessionID: "sess", RelatedRuntimeIDS: `["tool-a","tool-b"]`}).Error)
	flows := []*schema.HTTPFlow{
		{Hash: "flow-1", RuntimeId: "tool-b", Url: "https://example.test/login", Request: "GET /login HTTP/1.1", Response: "HTTP/1.1 200 OK"},
		{Hash: "flow-2", RuntimeId: "tool-b", Url: "https://other.test/login", Request: "GET /login"},
		{Hash: "flow-3", RuntimeId: "other", Url: "https://example.test/profile"},
		{Hash: "flow-4", RuntimeId: "tool-b", Url: "http://example.test/login"}, // wrong port
		{Hash: "flow-5", RuntimeId: "other", Url: "https://example.test/login", Request: "GET /login", Response: "HTTP/1.1 200 OK"},
	}
	for _, f := range flows {
		require.NoError(t, db.Create(f).Error)
	}
	env := &riskEnvironment{RiskID: int64(r.ID), RuntimeIDs: []string{"tool-a", "tool-b"}, Scope: scopeForRisk(r)}
	inventory, err := surveyEnvironment(db, env)
	require.NoError(t, err)
	require.Contains(t, inventory, "Total HTTP flows: 3")
	broader, _, _, err := searchFlows(db, r, env, searchCriteria{URLContains: "/login", RequestContains: "GET /login"})
	require.NoError(t, err)
	require.Len(t, broader, 2) // cross-target within the session is searchable; outside-session is not
	require.Equal(t, flows[1].ID, broader[0].Flow.ID)
	require.NotContains(t, broader[0].Why, "same host/port")
	require.NoError(t, attachFlow(db, env, int(flows[0].ID)))
	updated, err := loadRisk(db, int(r.ID))
	require.NoError(t, err)
	require.Len(t, updated.PacketPairs, 1)
	require.Equal(t, int64(flows[0].ID), updated.PacketPairs[0].HTTPFlowId)
	require.NotEmpty(t, updated.QuotedRequest)
	require.NoError(t, attachFlow(db, env, int(flows[0].ID))) // no duplicate
	updated, err = loadRisk(db, int(r.ID))
	require.NoError(t, err)
	require.Len(t, updated.PacketPairs, 1)
	require.Error(t, attachFlow(db, env, int(flows[2].ID)))                     // wrong endpoint, even on same host
	require.Error(t, attachFlow(db, env, int(flows[1].ID)))                     // unrelated host, same session
	require.ErrorContains(t, attachFlow(db, env, int(flows[4].ID)), "boundary") // exact endpoint does not bypass runtime boundary
}

func TestSearchFlowsUsesRiskCluesInsideSessionBoundary(t *testing.T) {
	db := riskTestDB(t)
	r := &schema.Risk{Hash: "search-risk", Host: "example.test", AISessionID: "session-1", RuntimeId: "tool-a",
		Title: "反射型 XSS", Parameter: "q", Payload: "<script>alert(1)</script>",
		Description: "搜索接口的 q 参数未经编码反射到 HTML", Details: strconv.Quote(`{"target_input":"https://example.test/search"}`)}
	require.NoError(t, db.Create(r).Error)
	require.NoError(t, db.Create(&schema.AISession{SessionID: "session-1", RelatedRuntimeIDS: `["tool-a","tool-b"]`}).Error)
	flows := []*schema.HTTPFlow{
		{Hash: "session-hit", RuntimeId: "tool-a", Url: "https://example.test/search?q=test", Method: "GET", StatusCode: 200,
			Request: "GET /search?q=%3Cscript%3Ealert(1)%3C/script%3E HTTP/1.1", Response: "HTTP/1.1 200 OK\r\n\r\nprobe-ok"},
		{Hash: "same-session-hit", RuntimeId: "tool-b", Url: "https://example.test/search?q=test", Method: "GET", StatusCode: 200,
			Request: "GET /search?q=%3Cscript%3Ealert(1)%3C/script%3E HTTP/1.1", Response: "HTTP/1.1 200 OK\r\n\r\nprobe-ok"},
		{Hash: "outside-session-hit", RuntimeId: "tool-other", Url: "https://example.test/search?q=test", Method: "GET", StatusCode: 200,
			Request: "GET /search?q=%3Cscript%3Ealert(1)%3C/script%3E HTTP/1.1", Response: "HTTP/1.1 200 OK\r\n\r\nprobe-ok"},
		{Hash: "same-session-wrong-target", RuntimeId: "tool-a", Url: "https://unrelated.test/search", Method: "GET", StatusCode: 200,
			Request: "GET /search?q=%3Cscript%3Ealert(1)%3C/script%3E HTTP/1.1", Response: "probe-ok"},
		{Hash: "same-host-wrong-port", RuntimeId: "tool-a", Url: "http://example.test/search", Method: "GET", StatusCode: 200,
			Request: "GET /search?q=%3Cscript%3Ealert(1)%3C/script%3E HTTP/1.1", Response: "probe-ok"},
	}
	for _, f := range flows {
		require.NoError(t, db.Create(f).Error)
	}
	q := searchCriteria{URLContains: "/search", RequestContains: "<script>alert(1)</script>", ResponseContains: "probe-ok", Method: "GET", StatusCode: 200}
	env := &riskEnvironment{RiskID: int64(r.ID), RuntimeIDs: []string{"tool-a", "tool-b"}, Scope: scopeForRisk(r)}
	got, _, more, err := searchFlows(db, r, env, q)
	require.NoError(t, err)
	require.False(t, more)
	require.Len(t, got, 4) // two same-target plus two cross-target, all inside the runtime boundary
	require.Equal(t, flows[4].ID, got[0].Flow.ID)
	env.RuntimeIDs = []string{"tool-a"}
	got, _, _, err = searchFlows(db, r, env, q)
	require.NoError(t, err)
	require.Len(t, got, 3)
	groups := groupFlowCandidates(got, 3)
	require.Len(t, groups, 3)
	context := riskContext(r)
	require.Contains(t, context, "反射型 XSS")
	require.Contains(t, context, "q 参数")
	require.Contains(t, context, "<script>alert(1)</script>")
	require.Contains(t, context, "path=/search")
}

func TestSearchFlowsFindsOldEvidenceAmongThousandsOfNewerFlows(t *testing.T) {
	db := riskTestDB(t)
	r := &schema.Risk{Hash: "older-risk", Host: "example.test", Parameter: "ticket", RuntimeId: "tool-a"}
	require.NoError(t, db.Create(r).Error)
	hit := &schema.HTTPFlow{Hash: "old-hit", RuntimeId: "tool-a", Url: "https://example.test/verify", Request: "GET /verify?ticket=secret"}
	require.NoError(t, db.Create(hit).Error)
	for i := 0; i < 1200; i++ {
		f := &schema.HTTPFlow{HiddenIndex: "noise-" + strconv.Itoa(i), RuntimeId: "tool-a", Url: "https://example.test/other"}
		require.NoError(t, db.Create(f).Error)
	}
	q := searchCriteria{RequestContains: "ticket=secret"}
	env := &riskEnvironment{RiskID: int64(r.ID), RuntimeIDs: []string{"tool-a"}, Scope: scopeForRisk(r)}
	got, _, more, err := searchFlows(db, r, env, q)
	require.NoError(t, err)
	require.False(t, more)
	require.Len(t, got, 1)
	require.Equal(t, hit.ID, got[0].Flow.ID)
}

func TestSearchFlowsUsesKeysetCursorOverMatchingRows(t *testing.T) {
	db := riskTestDB(t)
	r := &schema.Risk{Hash: "cursor-risk", Host: "example.test", RuntimeId: "runtime-cursor"}
	require.NoError(t, db.Create(r).Error)
	for i := 0; i < searchPageSize+5; i++ {
		f := &schema.HTTPFlow{HiddenIndex: "hit-" + strconv.Itoa(i), RuntimeId: r.RuntimeId,
			Url: "https://example.test/verify", Request: "GET /verify?proof=marker"}
		require.NoError(t, db.Create(f).Error)
	}
	env := &riskEnvironment{RiskID: int64(r.ID), RuntimeIDs: []string{r.RuntimeId}, Scope: scopeForRisk(r)}
	q := searchCriteria{RequestContains: "proof=marker"}
	first, cursor, more, err := searchFlows(db, r, env, q)
	require.NoError(t, err)
	require.Len(t, first, searchPageSize)
	require.True(t, more)
	groups := groupFlowCandidates(first, 12)
	require.Len(t, groups, 1)
	require.Equal(t, searchPageSize, groups[0].Count)
	q.BeforeID = cursor
	older, _, more, err := searchFlows(db, r, env, q)
	require.NoError(t, err)
	require.Len(t, older, 5)
	require.False(t, more)
	require.Less(t, older[0].Flow.ID, first[len(first)-1].Flow.ID)
}

func TestSearchFlowsUsesAttachedRiskCreationTime(t *testing.T) {
	db := riskTestDB(t)
	r := &schema.Risk{Hash: "time-risk", Host: "example.test", RuntimeId: "tool-time"}
	require.NoError(t, db.Create(r).Error)
	near := &schema.HTTPFlow{RuntimeId: r.RuntimeId, Url: "https://example.test/related"}
	old := &schema.HTTPFlow{RuntimeId: r.RuntimeId, Url: "https://example.test/older"}
	require.NoError(t, db.Create(near).Error)
	require.NoError(t, db.Create(old).Error)
	require.NoError(t, db.Model(old).UpdateColumn("created_at", r.CreatedAt.Add(-2*time.Hour)).Error)
	env := &riskEnvironment{RiskID: int64(r.ID), RuntimeIDs: []string{r.RuntimeId}, Scope: scopeForRisk(r)}
	hits, _, _, err := searchFlows(db, r, env, searchCriteria{TimeWindowMinutes: 10})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, near.ID, hits[0].Flow.ID)
}

func TestSearchWithoutRuntimeBoundaryDoesNotScanTarget(t *testing.T) {
	db := riskTestDB(t)
	r := &schema.Risk{Hash: "unbound-risk", Host: "example.test"}
	require.NoError(t, db.Create(r).Error)
	f := &schema.HTTPFlow{RuntimeId: "unrelated", Url: "https://example.test/search", Request: "GET /search?token=secret"}
	require.NoError(t, db.Create(f).Error)
	ports, err := findPorts(db, r, nil)
	require.NoError(t, err)
	require.Empty(t, ports)
	_, _, _, err = searchFlows(db, r, nil, searchCriteria{RequestContains: "token=secret"})
	require.ErrorContains(t, err, "environment is unavailable")
	require.ErrorContains(t, attachFlow(db, nil, int(f.ID)), "boundary")
}

func TestLegacyAIHostPathStillDefinesTarget(t *testing.T) {
	r := &schema.Risk{Host: "example.test/login?next=%2F"}
	s := scopeForRisk(r)
	require.Equal(t, "example.test", s.host)
	require.Equal(t, "/login", s.path)
	require.Zero(t, s.port) // scheme was not recorded: do not guess it
}

func TestWriteActionsRequireExplicitUserIntent(t *testing.T) {
	require.False(t, userRequestedWrite("只展示风险关联的流量，不要写入"))
	require.False(t, userRequestedWrite("查询风险 123 的流量"))
	require.True(t, userRequestedWrite("补全风险 123 的流量和端口，并保存"))
}

func TestAttachFlowPreservesExistingEvidenceAndRejectsUnrelatedRuntime(t *testing.T) {
	db := riskTestDB(t)
	r := &schema.Risk{Hash: "legacy", Host: "example.test", RuntimeId: "tool-a", QuotedRequest: strconv.Quote("original")}
	require.NoError(t, db.Create(r).Error)
	env := &riskEnvironment{RiskID: int64(r.ID), RuntimeIDs: []string{r.RuntimeId}, Scope: scopeForRisk(r)}
	stranger := &schema.HTTPFlow{Hash: "stranger", RuntimeId: "tool-b", Url: "http://example.test/else"}
	require.NoError(t, db.Create(stranger).Error)
	require.Error(t, attachFlow(db, env, int(stranger.ID)))
	f := &schema.HTTPFlow{Hash: "related", RuntimeId: "tool-a", Url: "http://example.test/else", Request: "GET /else", Response: "HTTP/1.1 200 OK"}
	require.NoError(t, db.Create(f).Error)
	require.NoError(t, attachFlow(db, env, int(f.ID)))
	updated, err := loadRisk(db, int(r.ID))
	require.NoError(t, err)
	require.Equal(t, strconv.Quote("original"), updated.QuotedRequest)
	require.Empty(t, updated.QuotedResponse) // don't pair a new response with "original"
	require.Len(t, updated.PacketPairs, 1)
}

func TestFillPortOnlyMatchingHostAndEmptyPort(t *testing.T) {
	db := riskTestDB(t)
	r := &schema.Risk{Hash: "port-risk", Host: "example.test", RuntimeId: "tool-a"}
	require.NoError(t, db.Create(r).Error)
	env := &riskEnvironment{RiskID: int64(r.ID), RuntimeIDs: []string{r.RuntimeId}, Scope: scopeForRisk(r)}
	other := &schema.Port{Host: "other.test", Port: 22, RuntimeId: "tool-a"}
	require.NoError(t, db.Create(other).Error)
	require.Error(t, fillPort(db, env, int(other.ID)))
	outside := &schema.Port{Host: "example.test", Port: 8443, RuntimeId: "other-tool"}
	require.NoError(t, db.Create(outside).Error)
	require.ErrorContains(t, fillPort(db, env, int(outside.ID)), "boundary")
	ports, err := findPorts(db, r, env)
	require.NoError(t, err)
	require.Empty(t, ports)
	port := &schema.Port{Host: "example.test", Port: 8443, RuntimeId: "tool-a"}
	require.NoError(t, db.Create(port).Error)
	ports, err = findPorts(db, r, env)
	require.NoError(t, err)
	require.Len(t, ports, 1)
	require.NoError(t, fillPort(db, env, int(port.ID)))
	require.Error(t, fillPort(db, env, int(port.ID)))
	updated, err := loadRisk(db, int(r.ID))
	require.NoError(t, err)
	require.Equal(t, 8443, updated.Port)
}
