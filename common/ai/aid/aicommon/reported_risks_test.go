package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ---------------------------------------------------------------------------
// ReportedRiskStore: append + dedup
// ---------------------------------------------------------------------------

func TestReportedRiskStore_AppendFromRisk_Dedup(t *testing.T) {
	store := NewReportedRiskStore()

	risk1 := &schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
		Title:     "登录接口 SQL 注入",
		Severity:  "high",
	}
	added := store.AppendFromRisk(risk1)
	require.True(t, added)
	require.Len(t, store.Items, 1)

	// Same target + type + parameter → duplicate, not added.
	risk2 := &schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
		Title:     "登录页面 username 参数注入", // different wording
		Severity:  "high",
	}
	added = store.AppendFromRisk(risk2)
	require.False(t, added)
	require.Len(t, store.Items, 1)
}

func TestReportedRiskStore_AppendFromRisk_DifferentParamIsNew(t *testing.T) {
	store := NewReportedRiskStore()

	store.AppendFromRisk(&schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
		Title:     "SQL 注入 (username)",
		Severity:  "high",
	})
	added := store.AppendFromRisk(&schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "password", // different parameter → new finding
		Title:     "SQL 注入 (password)",
		Severity:  "high",
	})
	require.True(t, added)
	require.Len(t, store.Items, 2)
}

func TestReportedRiskStore_AppendFromRisk_TargetNormalizationDedup(t *testing.T) {
	store := NewReportedRiskStore()

	// First report with full URL.
	store.AppendFromRisk(&schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
		Title:     "SQL 注入",
		Severity:  "high",
	})

	// Second report with trailing slash, default port, uppercase scheme —
	// after normalization the target is the same → duplicate.
	added := store.AppendFromRisk(&schema.Risk{
		Url:       "https://example.com:443/login/",
		RiskType:  "sqli",
		Parameter: "username",
		Title:     "SQL 注入 (re-verify)",
		Severity:  "high",
	})
	require.False(t, added, "normalized target should dedup")
	require.Len(t, store.Items, 1)
}

func TestReportedRiskStore_AppendFromRisk_DifferentTypeIsNew(t *testing.T) {
	store := NewReportedRiskStore()

	store.AppendFromRisk(&schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
		Title:     "SQL 注入",
		Severity:  "high",
	})

	// Same target + same param, but different type → new finding.
	added := store.AppendFromRisk(&schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "auth-bypass",
		Parameter: "username",
		Title:     "认证绕过",
		Severity:  "high",
	})
	require.True(t, added)
	require.Len(t, store.Items, 2)
}

func TestReportedRiskStore_AppendFromRisk_NilRisk(t *testing.T) {
	store := NewReportedRiskStore()
	require.False(t, store.AppendFromRisk(nil))
	require.Empty(t, store.Items)
}

// ---------------------------------------------------------------------------
// ReportedRiskStore: marshal / unmarshal round-trip
// ---------------------------------------------------------------------------

func TestReportedRiskStore_MarshalUnmarshalRoundTrip(t *testing.T) {
	store := NewReportedRiskStore()
	store.AppendFromRisk(&schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
		Title:     "登录接口 SQL 注入",
		Severity:  "high",
	})
	store.AppendFromRisk(&schema.Risk{
		Url:       "https://example.com/search",
		RiskType:  "xss",
		Parameter: "q",
		Title:     "反射型 XSS",
		Severity:  "high",
	})

	jsonStr := store.Marshal()
	require.NotEmpty(t, jsonStr)

	restored := UnmarshalReportedRiskStore(jsonStr)
	require.Len(t, restored.Items, 2)
	require.Equal(t, "sqli", restored.Items[0].RiskType)
	require.Equal(t, "xss", restored.Items[1].RiskType)
}

func TestUnmarshalReportedRiskStore_EmptyAndInvalid(t *testing.T) {
	require.Empty(t, UnmarshalReportedRiskStore("").Items)
	require.Empty(t, UnmarshalReportedRiskStore("not json").Items)
}

// ---------------------------------------------------------------------------
// normalizeRiskTargetForSummary
// ---------------------------------------------------------------------------

func TestNormalizeRiskTargetForSummary(t *testing.T) {
	tests := []struct {
		name string
		url  string
		host string
		port int
		want string
	}{
		{"strip https", "https://example.com/login", "", 0, "example.com/login"},
		{"strip http", "http://example.com/login", "", 0, "example.com/login"},
		{"strip default 443", "https://example.com:443/login", "", 0, "example.com/login"},
		{"strip default 80", "http://example.com:80/login", "", 0, "example.com/login"},
		{"strip trailing slash", "https://example.com/login/", "", 0, "example.com/login"},
		{"strip root slash", "https://example.com/", "", 0, "example.com"},
		{"fallback to host:port", "", "192.168.1.10", 22, "192.168.1.10:22"},
		{"fallback host no port", "", "example.com", 0, "example.com"},
		{"fallback host default port stripped", "", "example.com", 443, "example.com"},
		{"empty everything", "", "", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRiskTargetForSummary(tt.url, tt.host, tt.port)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeRiskTargetForSummary_TruncatesLongURL(t *testing.T) {
	longPath := strings.Repeat("a", 100)
	got := normalizeRiskTargetForSummary("https://example.com/"+longPath, "", 0)
	require.LessOrEqual(t, len([]rune(got)), 63) // 60 + "..."
	require.True(t, strings.HasPrefix(got, "example.com/"))
	require.True(t, strings.HasSuffix(got, "..."))
}

// ---------------------------------------------------------------------------
// computeReportedRiskID: determinism
// ---------------------------------------------------------------------------

func TestComputeReportedRiskID_Deterministic(t *testing.T) {
	id1 := computeReportedRiskID("example.com/login", "sqli", "username")
	id2 := computeReportedRiskID("example.com/login", "sqli", "username")
	require.Equal(t, id1, id2)

	// Different param → different ID.
	id3 := computeReportedRiskID("example.com/login", "sqli", "password")
	require.NotEqual(t, id1, id3)

	// Case-insensitive (normalized).
	id4 := computeReportedRiskID("EXAMPLE.COM/LOGIN", "SQLI", "Username")
	require.Equal(t, id1, id4)
}

// ---------------------------------------------------------------------------
// SessionPromptState: AppendReportedRisk
// ---------------------------------------------------------------------------

func TestSessionPromptState_AppendReportedRisk(t *testing.T) {
	s := NewSessionPromptState()

	// First risk → added.
	added := s.AppendReportedRisk(&schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
		Title:     "SQL 注入",
		Severity:  "high",
	})
	require.True(t, added)

	// Duplicate → not added.
	added = s.AppendReportedRisk(&schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
		Title:     "SQL 注入 (retry)",
		Severity:  "high",
	})
	require.False(t, added)

	require.JSONEq(t, `{"items":[{"id":"`+computeReportedRiskID("example.com/login", "sqli", "username")+
		`","severity":"high","risk_type":"sqli","target":"example.com/login","parameter":"username","title":"SQL 注入","created_unix":`+
		fmt.Sprint(s.reportedRiskStore.Items[0].CreatedUnix)+`}]}`, s.GetReportedRisks())
}

func TestSessionPromptState_AppendReportedRisk_Nil(t *testing.T) {
	s := NewSessionPromptState()
	require.False(t, s.AppendReportedRisk(nil))
	require.Empty(t, UnmarshalReportedRiskStore(s.GetReportedRisks()).Items)
}

// ---------------------------------------------------------------------------
// SessionPromptState: persistence (Get/Set)
// ---------------------------------------------------------------------------

func TestSessionPromptState_ReportedRisksPersistence(t *testing.T) {
	s := NewSessionPromptState()
	s.AppendReportedRisk(&schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
		Title:     "SQL 注入",
		Severity:  "high",
	})

	jsonStr := s.GetReportedRisks()
	require.NotEmpty(t, jsonStr)

	// Restore into a new state.
	s2 := NewSessionPromptState()
	s2.SetReportedRisks(jsonStr)
	require.JSONEq(t, jsonStr, s2.GetReportedRisks())
}

// ---------------------------------------------------------------------------
// SessionPromptState: ForkForSubAgent inherits reported risks
// ---------------------------------------------------------------------------

func TestSessionPromptState_ForkForSubAgent_SharesReportedRisks(t *testing.T) {
	parent := NewSessionPromptState()
	parent.AppendReportedRisk(&schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
		Title:     "SQL 注入",
		Severity:  "high",
	})

	child := parent.ForkForSubAgent()

	// Child shares the parent's risk ledger.
	require.Contains(t, child.GetReportedRisks(), "sqli")
	require.Contains(t, child.GetReportedRisks(), "example.com/login")

	// Child adds its own risk.
	added := child.AppendReportedRisk(&schema.Risk{
		Url:       "https://example.com/search",
		RiskType:  "xss",
		Parameter: "q",
		Title:     "XSS",
		Severity:  "high",
	})
	require.True(t, added)

	// Parent immediately sees the child's new risk — shared store, not copy.
	require.Contains(t, parent.GetReportedRisks(), "xss", "parent should see child's risk (shared store)")

	// Child still sees both.
	require.Contains(t, child.GetReportedRisks(), "sqli")
	require.Contains(t, child.GetReportedRisks(), "xss")

	// Child deduplicates against parent's risks.
	dup := child.AppendReportedRisk(&schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
		Title:     "SQL 注入 (child retry)",
		Severity:  "high",
	})
	require.False(t, dup, "child should dedup against shared parent risks")

	// Parent adding a risk is also visible to child.
	parent.AppendReportedRisk(&schema.Risk{
		Url:       "https://example.com/admin",
		RiskType:  "rce",
		Parameter: "cmd",
		Title:     "RCE",
		Severity:  "high",
	})
	require.Contains(t, child.GetReportedRisks(), "rce", "child should see parent's new risk (shared store)")
}

func TestSessionPromptState_ForkForSubAgent_SharesInitiallyEmptyReportedRisks(t *testing.T) {
	parent := NewSessionPromptState()
	child := parent.ForkForSubAgent()
	sibling := parent.ForkForSubAgent()
	risk := &schema.Risk{Url: "https://example.com/child-first", RiskType: "xss", Parameter: "q", Title: "child-first", Severity: "high"}
	require.True(t, child.AppendReportedRisk(risk))
	require.Contains(t, parent.GetReportedRisks(), "child-first")
	require.Contains(t, sibling.GetReportedRisks(), "child-first")
	require.False(t, sibling.AppendReportedRisk(risk), "deduplication must work even when the first risk comes from a child")
}

// ---------------------------------------------------------------------------
// Prompt injection: immutable reports appear at their Timeline positions
// ---------------------------------------------------------------------------

func TestReportedRisks_PromptPlacement_ThroughTimeline(t *testing.T) {
	cfg := newTimelineBroadcastConfig(t)
	require.True(t, cfg.AppendReportedRisk(&schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
		Title:     "登录接口 SQL 注入",
		Severity:  "high",
	}))

	frozenOpen := BuildPromptFrozenOpenMaterials(cfg, "ns-risk")
	require.Contains(t, frozenOpen.TimelineOpen, "sqli")
	require.Empty(t, frozenOpen.PromotedReportedRisks)

	materials := &PromptMaterials{
		TaskInstruction: "instruction",
		Schema:          `{"type":"object"}`,
	}
	ApplyPromptFrozenOpenMaterials(materials, frozenOpen)
	require.Contains(t, materials.TimelineOpen, "sqli")

	prompt, err := NewDefaultPromptPrefixBuilder().AssemblePromptWithDynamicSection(
		materials,
		"reported-risks-placement-dynamic",
		"dynamic",
		nil,
		"ns-risk",
	)
	require.NoError(t, err)

	// ReportedRisks content should appear in the prompt.
	require.Contains(t, prompt, "## 已上报风险（Reported Risks）")
	require.NotContains(t, prompt, "## 已报告漏洞清单")
	require.Contains(t, prompt, "sqli")
	require.Contains(t, prompt, "example.com/login")

	// Use aiprojection.Split to find section boundaries.
	sections := promptBuilderChunksBySection(t, prompt)
	timelineOpenChunks := sections[aiprojection.SectionTimelineOpen]
	require.NotEmpty(t, timelineOpenChunks, "timeline-open section should exist")

	// The ReportedRisks content should be inside the timeline-open section.
	var tlOpenContent string
	for _, ch := range timelineOpenChunks {
		tlOpenContent += ch.Content
	}
	require.Contains(t, tlOpenContent, "## 已上报风险（Reported Risks）")
	require.Contains(t, tlOpenContent, "sqli")
}

func TestReportedRisks_PromptPlacement_EmptyWhenNoRisks(t *testing.T) {
	cfg := newTimelineBroadcastConfig(t)

	frozenOpen := BuildPromptFrozenOpenMaterials(cfg, "ns-empty")
	require.Empty(t, frozenOpen.PromotedReportedRisks)

	materials := &PromptMaterials{
		TaskInstruction: "instruction",
		Schema:          `{"type":"object"}`,
	}
	ApplyPromptFrozenOpenMaterials(materials, frozenOpen)

	prompt, err := NewDefaultPromptPrefixBuilder().AssemblePromptWithDynamicSection(
		materials,
		"empty-risks-dynamic",
		"dynamic",
		nil,
		"ns-empty",
	)
	require.NoError(t, err)

	require.NotContains(t, prompt, "## 已报告漏洞清单")
}

// ---------------------------------------------------------------------------
// Config: interface methods
// ---------------------------------------------------------------------------

func TestConfig_ReportedRisksMethods(t *testing.T) {
	cfg := NewConfig(context.Background())

	// Initially empty.
	require.Empty(t, cfg.GetReportedRisks())

	// Append a risk.
	added := cfg.AppendReportedRisk(&schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
		Title:     "SQL 注入",
		Severity:  "high",
	})
	require.True(t, added)

	require.Contains(t, cfg.GetReportedRisks(), "sqli")
	require.NotEmpty(t, cfg.GetReportedRisks())

	// Restore from JSON.
	jsonStr := cfg.GetReportedRisks()
	cfg2 := NewConfig(context.Background())
	cfg2.SetReportedRisks(jsonStr)
	require.Contains(t, cfg2.GetReportedRisks(), "sqli")
}

// ---------------------------------------------------------------------------
// handleRiskMessage: Parameter and Payload parsing
// ---------------------------------------------------------------------------

func TestHandleRiskMessage_ParsesParameterAndPayload(t *testing.T) {
	// Build a json-risk Yakit log message with Parameter and Payload fields.
	riskData := map[string]any{
		"Title":     "SQL 注入",
		"Url":       "https://example.com/login",
		"RiskType":  "sqli",
		"Severity":  "high",
		"Parameter": "username",
		"Payload":   "' OR '1'='1",
		"Hash":      "test-hash",
		"RuntimeId": "rt-1",
	}
	riskBytes, err := json.Marshal(riskData)
	require.NoError(t, err)

	logInfo := map[string]any{
		"level": "json-risk",
		"data":  string(riskBytes),
	}
	logBytes, err := json.Marshal(logInfo)
	require.NoError(t, err)

	// Build YakitMessage: {"type":"log","content": <raw json of log>}
	msgBytes, err := json.Marshal(map[string]json.RawMessage{
		"type":    json.RawMessage(`"log"`),
		"content": json.RawMessage(logBytes),
	})
	require.NoError(t, err)

	result := &ypb.ExecResult{
		IsMessage: true,
		Message:   msgBytes,
	}

	risk, err := handleRiskMessage(result)
	require.NoError(t, err)
	require.NotNil(t, risk)
	require.Equal(t, "username", risk.Parameter)
	require.Equal(t, "' OR '1'='1", risk.Payload)
	require.Equal(t, "sqli", risk.RiskType)
}

// ---------------------------------------------------------------------------
// handleRiskMessage: returns error for non-risk messages
// ---------------------------------------------------------------------------

func TestHandleRiskMessage_NonRiskMessage(t *testing.T) {
	logInfo := map[string]any{
		"level": "info",
		"data":  "some log",
	}
	logBytes, _ := json.Marshal(logInfo)

	msgBytes, _ := json.Marshal(map[string]json.RawMessage{
		"type":    json.RawMessage(`"log"`),
		"content": json.RawMessage(logBytes),
	})

	result := &ypb.ExecResult{
		IsMessage: true,
		Message:   msgBytes,
	}

	risk, err := handleRiskMessage(result)
	require.Error(t, err)
	require.Nil(t, risk)
}
