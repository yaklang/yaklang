package loop_intent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// --- Unit tests for capability_enrichment.go ---

func TestBuildCapabilityEnrichmentMarkdown_AllFourTypes(t *testing.T) {
	details := []capabilityDetail{
		{CapabilityName: "synscan", CapabilityType: "tool", Description: "SYN port scanning tool"},
		{CapabilityName: "servicescan", CapabilityType: "tool", Description: "Service fingerprint detection"},
		{CapabilityName: "report_gen", CapabilityType: "forge", Description: "Generate penetration test reports"},
		{CapabilityName: "nuclei_scan", CapabilityType: "skill", Description: "Nuclei vulnerability scanning skill"},
		{CapabilityName: "pentest_mode", CapabilityType: "focus_mode", Description: "Full penetration testing workflow"},
	}

	md := buildCapabilityEnrichmentMarkdown(details, nil)
	if md == "" {
		t.Fatal("expected non-empty Markdown output")
	}

	// Verify header
	if !strings.Contains(md, "Recommended Capabilities") {
		t.Fatal("missing 'Recommended Capabilities' header")
	}

	// Verify all 4 type sections
	for _, capType := range capabilityTypeOrder {
		label := capabilityTypeLabels[capType]
		if !strings.Contains(md, label) {
			t.Fatalf("missing section label for type %q: %s", capType, label)
		}
		guide := capabilityTypeUsageGuides[capType]
		if !strings.Contains(md, guide) {
			t.Fatalf("missing usage guide for type %q", capType)
		}
	}

	// Verify tool usage guide mentions require_tool
	if !strings.Contains(md, "require_tool") {
		t.Fatal("tool section should mention require_tool")
	}
	// Verify forge usage guide mentions tool_compose
	if !strings.Contains(md, "tool_compose") {
		t.Fatal("forge section should mention tool_compose")
	}
	// Verify focus_mode usage guide mentions enter_focus_mode
	if !strings.Contains(md, "enter_focus_mode") {
		t.Fatal("focus_mode section should mention enter_focus_mode")
	}

	// Verify all capability names are present
	for _, d := range details {
		if !strings.Contains(md, d.CapabilityName) {
			t.Fatalf("missing capability name %q in output", d.CapabilityName)
		}
		if !strings.Contains(md, d.Description) {
			t.Fatalf("missing description for %q in output", d.CapabilityName)
		}
	}

	t.Logf("generated Markdown:\n%s", md)
}

func TestBuildCapabilityEnrichmentMarkdown_FilterByRecommended(t *testing.T) {
	details := []capabilityDetail{
		{CapabilityName: "synscan", CapabilityType: "tool", Description: "SYN port scanner"},
		{CapabilityName: "servicescan", CapabilityType: "tool", Description: "Service detection"},
		{CapabilityName: "report_gen", CapabilityType: "forge", Description: "Report generator"},
		{CapabilityName: "pentest_mode", CapabilityType: "focus_mode", Description: "Pentest workflow"},
	}

	recommended := map[string]bool{
		"synscan":    true,
		"report_gen": true,
	}

	md := buildCapabilityEnrichmentMarkdown(details, recommended)
	if md == "" {
		t.Fatal("expected non-empty Markdown output")
	}

	if !strings.Contains(md, "synscan") {
		t.Fatal("recommended capability 'synscan' should be present")
	}
	if !strings.Contains(md, "report_gen") {
		t.Fatal("recommended capability 'report_gen' should be present")
	}
	if strings.Contains(md, "servicescan") {
		t.Fatal("non-recommended capability 'servicescan' should be excluded")
	}
	if strings.Contains(md, "pentest_mode") {
		t.Fatal("non-recommended capability 'pentest_mode' should be excluded")
	}

	// Should have tool and forge sections, but NOT focus_mode
	if !strings.Contains(md, capabilityTypeLabels["tool"]) {
		t.Fatal("tool section should be present")
	}
	if !strings.Contains(md, capabilityTypeLabels["forge"]) {
		t.Fatal("forge section should be present")
	}
	if strings.Contains(md, capabilityTypeLabels["focus_mode"]) {
		t.Fatal("focus_mode section should not be present when no focus_mode is recommended")
	}
}

func TestBuildCapabilityEnrichmentMarkdown_EmptyInput(t *testing.T) {
	md := buildCapabilityEnrichmentMarkdown(nil, nil)
	if md != "" {
		t.Fatalf("expected empty string for nil input, got: %s", md)
	}

	md = buildCapabilityEnrichmentMarkdown([]capabilityDetail{}, nil)
	if md != "" {
		t.Fatalf("expected empty string for empty input, got: %s", md)
	}
}

func TestBuildCapabilityEnrichmentMarkdown_TypeOrdering(t *testing.T) {
	details := []capabilityDetail{
		{CapabilityName: "fm1", CapabilityType: "focus_mode", Description: "focus mode first"},
		{CapabilityName: "sk1", CapabilityType: "skill", Description: "skill second"},
		{CapabilityName: "fg1", CapabilityType: "forge", Description: "forge third"},
		{CapabilityName: "tl1", CapabilityType: "tool", Description: "tool last"},
	}

	md := buildCapabilityEnrichmentMarkdown(details, nil)

	toolIdx := strings.Index(md, capabilityTypeLabels["tool"])
	forgeIdx := strings.Index(md, capabilityTypeLabels["forge"])
	skillIdx := strings.Index(md, capabilityTypeLabels["skill"])
	focusIdx := strings.Index(md, capabilityTypeLabels["focus_mode"])

	if toolIdx > forgeIdx {
		t.Fatal("tool section should appear before forge section")
	}
	if forgeIdx > skillIdx {
		t.Fatal("forge section should appear before skill section")
	}
	if skillIdx > focusIdx {
		t.Fatal("skill section should appear before focus_mode section")
	}
}

// --- JSON round-trip tests ---

func TestCapabilityDetailsJSONRoundTrip(t *testing.T) {
	original := []capabilityDetail{
		{CapabilityName: "synscan", CapabilityType: "tool", Description: "SYN port scanner"},
		{CapabilityName: "report_gen", CapabilityType: "forge", Description: "Report generator"},
		{CapabilityName: "nuclei_skill", CapabilityType: "skill", Description: "Nuclei scanning skill"},
		{CapabilityName: "pentest_mode", CapabilityType: "focus_mode", Description: "Pentest workflow"},
	}

	jsonStr := marshalCapabilityDetails(original)
	if jsonStr == "" {
		t.Fatal("marshalCapabilityDetails returned empty string")
	}

	// Verify valid JSON
	var rawCheck []map[string]string
	if err := json.Unmarshal([]byte(jsonStr), &rawCheck); err != nil {
		t.Fatalf("JSON output is not valid: %v", err)
	}
	if len(rawCheck) != 4 {
		t.Fatalf("expected 4 items in JSON array, got %d", len(rawCheck))
	}

	// Verify JSON field names
	for _, item := range rawCheck {
		if _, ok := item["capability_name"]; !ok {
			t.Fatal("JSON item missing 'capability_name' field")
		}
		if _, ok := item["capability_type"]; !ok {
			t.Fatal("JSON item missing 'capability_type' field")
		}
		if _, ok := item["description"]; !ok {
			t.Fatal("JSON item missing 'description' field")
		}
	}

	// Round-trip parse
	parsed := parseCapabilityDetails(jsonStr)
	if len(parsed) != len(original) {
		t.Fatalf("round-trip: expected %d items, got %d", len(original), len(parsed))
	}
	for i, p := range parsed {
		if p.CapabilityName != original[i].CapabilityName {
			t.Fatalf("round-trip: item %d name mismatch: %q vs %q", i, p.CapabilityName, original[i].CapabilityName)
		}
		if p.CapabilityType != original[i].CapabilityType {
			t.Fatalf("round-trip: item %d type mismatch: %q vs %q", i, p.CapabilityType, original[i].CapabilityType)
		}
		if p.Description != original[i].Description {
			t.Fatalf("round-trip: item %d description mismatch", i)
		}
	}
}

func TestCapabilityDetailsJSON_EmptyAndNil(t *testing.T) {
	if s := marshalCapabilityDetails(nil); s != "" {
		t.Fatalf("expected empty string for nil, got: %s", s)
	}
	if s := marshalCapabilityDetails([]capabilityDetail{}); s != "" {
		t.Fatalf("expected empty string for empty slice, got: %s", s)
	}
	if p := parseCapabilityDetails(""); p != nil {
		t.Fatalf("expected nil for empty string, got: %v", p)
	}
	if p := parseCapabilityDetails("invalid json"); p != nil {
		t.Fatalf("expected nil for invalid JSON, got: %v", p)
	}
}

// --- Focus mode search tests (searchLoopMetadata) ---

func TestSearchLoopMetadata_MatchesNonHiddenLoops(t *testing.T) {
	testName := "test_focus_search_" + utils.RandStringBytes(8)
	testDesc := "vulnerability scanning and penetration test focus mode " + testName

	err := reactloops.RegisterLoopFactory(
		testName,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			return nil, nil
		},
		reactloops.WithLoopDescription(testDesc),
		reactloops.WithLoopIsHidden(false),
	)
	if err != nil {
		t.Fatalf("failed to register test loop: %v", err)
	}

	results := searchLoopMetadata(testName)
	found := false
	for _, meta := range results {
		if meta.Name == testName {
			found = true
			if meta.Description != testDesc {
				t.Fatalf("description mismatch: %q vs %q", meta.Description, testDesc)
			}
			break
		}
	}
	if !found {
		t.Fatalf("searchLoopMetadata did not find registered loop %q", testName)
	}
}

func TestSearchLoopMetadata_ExcludesHiddenLoops(t *testing.T) {
	testName := "test_hidden_loop_" + utils.RandStringBytes(8)

	err := reactloops.RegisterLoopFactory(
		testName,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			return nil, nil
		},
		reactloops.WithLoopDescription("hidden loop "+testName),
		reactloops.WithLoopIsHidden(true),
	)
	if err != nil {
		t.Fatalf("failed to register test loop: %v", err)
	}

	results := searchLoopMetadata(testName)
	for _, meta := range results {
		if meta.Name == testName {
			t.Fatalf("hidden loop %q should not appear in search results", testName)
		}
	}
}

func TestSearchLoopMetadata_TokenLevelMatch(t *testing.T) {
	testName := "test_token_match_" + utils.RandStringBytes(8)
	testDesc := "security assessment vulnerability scanning " + testName

	err := reactloops.RegisterLoopFactory(
		testName,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			return nil, nil
		},
		reactloops.WithLoopDescription(testDesc),
		reactloops.WithLoopIsHidden(false),
	)
	if err != nil {
		t.Fatalf("failed to register test loop: %v", err)
	}

	results := searchLoopMetadata("security vulnerability " + testName)
	found := false
	for _, meta := range results {
		if meta.Name == testName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("token-level search should find loop %q with multi-keyword query", testName)
	}
}

// --- Skill search tests (direct function test) ---

func TestSkillSearchMatching(t *testing.T) {
	nonce := utils.RandStringBytes(8)
	skills := []*aiskillloader.SkillMeta{
		{Name: "nuclei_scan_" + nonce, Description: "Nuclei vulnerability scanning for web applications " + nonce},
		{Name: "sqlmap_skill_" + nonce, Description: "SQL injection detection and exploitation " + nonce},
		{Name: "unrelated_skill_" + nonce, Description: "Something completely different " + nonce},
	}

	query := "nuclei vulnerability scanning " + nonce
	queryLower := strings.ToLower(query)
	queryTokens := strings.Fields(queryLower)

	var matched []string
	for _, meta := range skills {
		searchText := strings.ToLower(meta.Name + " " + meta.Description)

		if strings.Contains(searchText, queryLower) {
			matched = append(matched, meta.Name)
			continue
		}

		if len(queryTokens) > 1 {
			meaningfulTokens := 0
			matchCount := 0
			for _, token := range queryTokens {
				if len(token) < 2 {
					continue
				}
				meaningfulTokens++
				if strings.Contains(searchText, token) {
					matchCount++
				}
			}
			if meaningfulTokens > 0 && matchCount > 0 && matchCount >= (meaningfulTokens+1)/2 {
				matched = append(matched, meta.Name)
			}
		}
	}

	if len(matched) == 0 {
		t.Fatal("expected at least one skill to match")
	}
	if matched[0] != "nuclei_scan_"+nonce {
		t.Fatalf("expected first match to be nuclei_scan_%s, got %s", nonce, matched[0])
	}

	foundUnrelated := false
	for _, name := range matched {
		if name == "unrelated_skill_"+nonce {
			foundUnrelated = true
		}
	}
	if foundUnrelated {
		t.Fatal("unrelated_skill should not match the nuclei vulnerability scanning query")
	}
}

// --- BM25 tool/forge search tests (DB-dependent) ---

func TestSearchBM25_ToolAndForge(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Skip("profile database not available, skipping BM25 search test")
	}

	nonce := utils.RandStringBytes(12)
	toolName := "test_tool_intent_" + nonce
	forgeName := "test_forge_intent_" + nonce

	// Seed test tool
	_, err := yakit.CreateAIYakTool(db, &schema.AIYakTool{
		Name:        toolName,
		VerboseName: "Test Tool " + nonce,
		Description: "A test tool for intent recognition search verification " + nonce,
		Keywords:    "security scanning " + nonce,
	})
	if err != nil {
		t.Fatalf("failed to create test tool: %v", err)
	}
	defer yakit.DeleteAIYakTools(db, toolName)

	// Seed test forge
	yakit.CreateAIForge(db, &schema.AIForge{
		ForgeName:        forgeName,
		ForgeVerboseName: "Test Forge " + nonce,
		Description:      "A test forge for intent recognition search verification " + nonce,
		ForgeType:        "liteforge",
		ForgeContent:     `{"params": [], "plan": "echo test"}`,
	})
	defer yakit.DeleteAIForgeByName(db, forgeName)

	// Search tools via BM25
	tools, err := yakit.SearchAIYakToolBM25(db, &yakit.AIYakToolFilter{
		Keywords: nonce,
	}, 10, 0)
	if err != nil {
		t.Fatalf("BM25 tool search failed: %v", err)
	}

	toolFound := false
	for _, tool := range tools {
		if tool.Name == toolName {
			toolFound = true
			break
		}
	}
	if !toolFound {
		t.Fatalf("BM25 search did not find seeded tool %q (got %d results)", toolName, len(tools))
	}

	// Search forges via BM25
	forges, err := yakit.SearchAIForgeBM25(db, &yakit.AIForgeSearchFilter{
		Keywords: nonce,
	}, 10, 0)
	if err != nil {
		t.Fatalf("BM25 forge search failed: %v", err)
	}

	forgeFound := false
	for _, forge := range forges {
		if forge.ForgeName == forgeName {
			forgeFound = true
			break
		}
	}
	if !forgeFound {
		t.Fatalf("BM25 search did not find seeded forge %q (got %d results)", forgeName, len(forges))
	}

	// Build structured capability details from search results
	var capDetails []capabilityDetail
	for _, tool := range tools {
		if tool.Name == toolName {
			appendCapDetail(&capDetails, tool.Name, "tool", tool.Description)
		}
	}
	for _, forge := range forges {
		if forge.ForgeName == forgeName {
			appendCapDetail(&capDetails, forge.ForgeName, "forge", forge.Description)
		}
	}

	if len(capDetails) != 2 {
		t.Fatalf("expected 2 capability details (1 tool + 1 forge), got %d", len(capDetails))
	}

	// Verify JSON round-trip
	jsonStr := marshalCapabilityDetails(capDetails)
	parsed := parseCapabilityDetails(jsonStr)
	if len(parsed) != 2 {
		t.Fatalf("round-trip: expected 2 items, got %d", len(parsed))
	}

	// Verify Markdown generation
	md := buildCapabilityEnrichmentMarkdown(parsed, nil)
	if !strings.Contains(md, toolName) {
		t.Fatalf("Markdown should contain tool name %q", toolName)
	}
	if !strings.Contains(md, forgeName) {
		t.Fatalf("Markdown should contain forge name %q", forgeName)
	}
	if !strings.Contains(md, "require_tool") {
		t.Fatal("Markdown should contain require_tool usage guide for tool type")
	}
	if !strings.Contains(md, "tool_compose") {
		t.Fatal("Markdown should contain tool_compose usage guide for forge type")
	}

	t.Logf("BM25 search test passed: tool=%s, forge=%s", toolName, forgeName)
}

// --- End-to-end enrichment test combining all 4 types ---

func TestCapabilityEnrichment_AllFourTypesEndToEnd(t *testing.T) {
	nonce := utils.RandStringBytes(8)

	details := []capabilityDetail{
		{CapabilityName: "tool_" + nonce, CapabilityType: "tool", Description: "test tool " + nonce},
		{CapabilityName: "forge_" + nonce, CapabilityType: "forge", Description: "test forge " + nonce},
		{CapabilityName: "skill_" + nonce, CapabilityType: "skill", Description: "test skill " + nonce},
		{CapabilityName: "focus_" + nonce, CapabilityType: "focus_mode", Description: "test focus mode " + nonce},
	}

	// Marshal to JSON (simulating what action_search_capabilities stores)
	jsonStr := marshalCapabilityDetails(details)
	if jsonStr == "" {
		t.Fatal("marshal should produce non-empty JSON")
	}

	// Parse back (simulating what action_finalize reads)
	parsed := parseCapabilityDetails(jsonStr)
	if len(parsed) != 4 {
		t.Fatalf("expected 4 parsed details, got %d", len(parsed))
	}

	// Build unfiltered Markdown (all capabilities)
	mdAll := buildCapabilityEnrichmentMarkdown(parsed, nil)
	if mdAll == "" {
		t.Fatal("unfiltered Markdown should be non-empty")
	}
	for _, d := range details {
		if !strings.Contains(mdAll, d.CapabilityName) {
			t.Fatalf("unfiltered Markdown missing capability %q", d.CapabilityName)
		}
	}

	// Verify all 4 type labels and usage guides
	for _, capType := range capabilityTypeOrder {
		if !strings.Contains(mdAll, capabilityTypeLabels[capType]) {
			t.Fatalf("missing type label for %q", capType)
		}
		if !strings.Contains(mdAll, capabilityTypeUsageGuides[capType]) {
			t.Fatalf("missing usage guide for %q", capType)
		}
	}

	// Build filtered Markdown (only tool and forge recommended)
	recommended := map[string]bool{
		"tool_" + nonce:  true,
		"forge_" + nonce: true,
	}
	mdFiltered := buildCapabilityEnrichmentMarkdown(parsed, recommended)
	if mdFiltered == "" {
		t.Fatal("filtered Markdown should be non-empty")
	}
	if !strings.Contains(mdFiltered, "tool_"+nonce) {
		t.Fatal("filtered Markdown should contain recommended tool")
	}
	if !strings.Contains(mdFiltered, "forge_"+nonce) {
		t.Fatal("filtered Markdown should contain recommended forge")
	}
	if strings.Contains(mdFiltered, "skill_"+nonce) {
		t.Fatal("filtered Markdown should not contain non-recommended skill")
	}
	if strings.Contains(mdFiltered, "focus_"+nonce) {
		t.Fatal("filtered Markdown should not contain non-recommended focus mode")
	}

	t.Logf("end-to-end enrichment test passed with nonce=%s", nonce)
}
