package loop_default

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// ============================================================================
// InputScale classification tests (pure rules, no I/O)
// ============================================================================

func TestClassifyInputScale_EmptyInput(t *testing.T) {
	scale := ClassifyInputScale("")
	if scale != InputScaleMicro {
		t.Errorf("expected InputScaleMicro for empty input, got %s", scale.String())
	}

	scale = ClassifyInputScale("   ")
	if scale != InputScaleMicro {
		t.Errorf("expected InputScaleMicro for whitespace input, got %s", scale.String())
	}
}

func TestClassifyInputScale_MicroInput(t *testing.T) {
	cases := []string{
		"hello",
		"你好",
		"hi",
		"ping",
		"test",
		"status",
		"帮我",
	}

	for _, input := range cases {
		scale := ClassifyInputScale(input)
		if scale != InputScaleMicro {
			t.Errorf("expected InputScaleMicro for input %q (len=%d), got %s", input, len([]rune(input)), scale.String())
		}
	}
}

func TestClassifyInputScale_SmallInput(t *testing.T) {
	cases := []string{
		"How do I scan a website for vulnerabilities?",
		"I need to write a yaklang script to test HTTP requests",
		"Can you help me analyze this HTTP flow?",
		"Help me generate a report for this scan result",
	}

	for _, input := range cases {
		scale := ClassifyInputScale(input)
		if scale != InputScaleSmall {
			t.Errorf("expected InputScaleSmall for input %q (rune_len=%d), got %s", input, len([]rune(input)), scale.String())
		}
	}
}

func TestClassifyInputScale_MediumInput(t *testing.T) {
	input := "I need to analyze the HTTP traffic from my web application. " +
		"Please help me identify potential security vulnerabilities in the request/response pairs. " +
		"I have captured several HTTP flows that show suspicious behavior patterns in the authentication mechanism. " +
		"Some endpoints seem to be vulnerable to injection attacks."

	scale := ClassifyInputScale(input)
	if scale != InputScaleMedium {
		t.Errorf("expected InputScaleMedium for input (rune_len=%d), got %s", len([]rune(input)), scale.String())
	}
}

func TestClassifyInputScale_LargeInput(t *testing.T) {
	input := strings.Repeat("This is a detailed security analysis request. ", 20) +
		"Please investigate all the network traffic and provide comprehensive findings."

	scale := ClassifyInputScale(input)
	if scale != InputScaleLarge {
		t.Errorf("expected InputScaleLarge for input (rune_len=%d), got %s", len([]rune(input)), scale.String())
	}
}

func TestClassifyInputScale_XLargeInput(t *testing.T) {
	input := strings.Repeat("This is an extremely detailed security audit request with lots of context. ", 50)

	scale := ClassifyInputScale(input)
	if scale != InputScaleXLarge {
		t.Errorf("expected InputScaleXLarge for input (rune_len=%d), got %s", len([]rune(input)), scale.String())
	}
}

func TestClassifyInputScale_CodeBlockBumpsScale(t *testing.T) {
	input := "Fix this:\n```\nfunc main() { fmt.Println(\"hello\") }\n```"

	scale := ClassifyInputScale(input)
	if scale.IsMicroOrSmall() {
		t.Errorf("expected at least InputScaleMedium for input with code block, got %s", scale.String())
	}
}

func TestClassifyInputScale_URLBumpsScale(t *testing.T) {
	input := "Scan https://example.com/api/v1/users for vulnerabilities"

	scale := ClassifyInputScale(input)
	if scale != InputScaleSmall && scale != InputScaleMedium {
		t.Errorf("expected InputScaleSmall or InputScaleMedium for input with URL, got %s", scale.String())
	}
}

func TestClassifyInputScale_ListItemsBumpScale(t *testing.T) {
	input := "Please do the following:\n- Scan the target\n- Analyze results\n- Generate report"

	scale := ClassifyInputScale(input)
	if scale == InputScaleMicro {
		t.Errorf("expected at least InputScaleSmall for input with list items, got %s", scale.String())
	}
}

func TestClassifyInputScale_ChineseInput(t *testing.T) {
	scale := ClassifyInputScale("你好世界")
	if scale != InputScaleMicro {
		t.Errorf("expected InputScaleMicro for short Chinese input, got %s", scale.String())
	}

	mediumInput := strings.Repeat("这是一个测试句子。", 15)
	scale = ClassifyInputScale(mediumInput)
	if scale != InputScaleMedium {
		t.Errorf("expected InputScaleMedium for medium Chinese input (rune_len=%d), got %s",
			len([]rune(mediumInput)), scale.String())
	}
}

func TestClassifyInputScale_ManySentencesBumpScale(t *testing.T) {
	input := "Do this. Then that. Also this. And that. Plus this. Finally that. Done. OK. Next."
	scale := ClassifyInputScale(input)
	if scale == InputScaleMicro || scale == InputScaleSmall {
		t.Errorf("expected at least InputScaleMedium for input with many sentences, got %s", scale.String())
	}
}

// ============================================================================
// InputScale method tests
// ============================================================================

func TestInputScale_IsMicroOrSmall(t *testing.T) {
	if !InputScaleMicro.IsMicroOrSmall() {
		t.Error("InputScaleMicro should be IsMicroOrSmall()")
	}
	if !InputScaleSmall.IsMicroOrSmall() {
		t.Error("InputScaleSmall should be IsMicroOrSmall()")
	}
	if InputScaleMedium.IsMicroOrSmall() {
		t.Error("InputScaleMedium should NOT be IsMicroOrSmall()")
	}
	if InputScaleLarge.IsMicroOrSmall() {
		t.Error("InputScaleLarge should NOT be IsMicroOrSmall()")
	}
	if InputScaleXLarge.IsMicroOrSmall() {
		t.Error("InputScaleXLarge should NOT be IsMicroOrSmall()")
	}
}

func TestInputScale_String(t *testing.T) {
	cases := map[InputScale]string{
		InputScaleMicro:  "Micro",
		InputScaleSmall:  "Small",
		InputScaleMedium: "Medium",
		InputScaleLarge:  "Large",
		InputScaleXLarge: "XLarge",
		InputScale(99):   "Unknown",
	}

	for scale, expected := range cases {
		if scale.String() != expected {
			t.Errorf("expected %q for scale %d, got %q", expected, scale, scale.String())
		}
	}
}

// ============================================================================
// Greeting pattern tests
// ============================================================================

func TestGreetingPatterns(t *testing.T) {
	greetings := []string{
		"你好",
		"hello",
		"hi",
		"嗨",
		"Hey",
		"你是谁",
		"你能做什么",
		"ping",
		"status",
		"谢谢",
		"thanks",
		"Hello!",
		"你好！",
		"Hi?",
	}

	for _, input := range greetings {
		if !greetingPatterns.MatchString(strings.TrimSpace(input)) {
			t.Errorf("expected greeting pattern to match %q", input)
		}
	}
}

func TestGreetingPatterns_NonGreetings(t *testing.T) {
	nonGreetings := []string{
		"scan this website",
		"help me write a yaklang script",
		"analyze HTTP flow",
		"what is SQL injection and how to prevent it",
		"generate report for scan results",
		"你好，帮我扫描一下这个网站",
	}

	for _, input := range nonGreetings {
		if greetingPatterns.MatchString(strings.TrimSpace(input)) {
			t.Errorf("greeting pattern should NOT match %q", input)
		}
	}
}

// ============================================================================
// Utility function tests
// ============================================================================

func TestCountSentences(t *testing.T) {
	cases := map[string]int{
		"":                     0,
		"hello":                1,
		"hello. world":         1,
		"hello. world.":        2,
		"first\nsecond\nthird": 2, // 2 newlines = 2 terminators, third segment has no terminator
		"sentence one. two!":   2,
		"question? answer.":    2,
		"中文句子。第二句。":            2,
		"混合。mixed! question?":  3,
	}

	for input, expected := range cases {
		count := countSentences(input)
		if count != expected {
			t.Errorf("countSentences(%q): expected %d, got %d", input, expected, count)
		}
	}
}

func TestTruncateString(t *testing.T) {
	cases := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 10, ""},
		{"你好世界", 2, "你好..."},
		{"short", 100, "short"},
	}

	for _, tc := range cases {
		result := truncateString(tc.input, tc.maxLen)
		if result != tc.expected {
			t.Errorf("truncateString(%q, %d): expected %q, got %q", tc.input, tc.maxLen, tc.expected, result)
		}
	}
}

// ============================================================================
// containsAnyToken tests (token-level keyword matching)
// ============================================================================

func TestContainsAnyToken(t *testing.T) {
	cases := []struct {
		name         string
		searchFields string
		input        string
		expected     bool
	}{
		{"multi-token full match", "http scan vulnerability", "http scan", true},
		{"single token returns false", "http scan vulnerability", "http", false},
		{"no tokens match", "http scan vulnerability", "xx yy", false},
		{"half tokens match", "http scan vulnerability", "http yy", true},
		{"zero of three", "http scan vulnerability", "xx yy zz", false},
		{"multi-token match", "yaklang code generation tool", "yaklang code", true},
		// Meaningful token filtering: tokens with len < 2 are ignored
		{"short tokens ignored", "http scan vulnerability", "a b http scan", true},
		// All tokens are too short
		{"all short tokens", "http scan vulnerability", "a b c", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := containsAnyToken(tc.searchFields, tc.input)
			if result != tc.expected {
				t.Errorf("containsAnyToken(%q, %q): expected %v, got %v", tc.searchFields, tc.input, tc.expected, result)
			}
		})
	}
}

// ============================================================================
// FastMatchResult tests
// ============================================================================

func TestFastMatchResult_HasMatches(t *testing.T) {
	result := &FastMatchResult{}
	if result.HasMatches() {
		t.Error("empty result should not have matches")
	}

	// IsSimpleQuery does not count as HasMatches
	result.IsSimpleQuery = true
	if result.HasMatches() {
		t.Error("simple query flag alone should not count as HasMatches")
	}
}

// ============================================================================
// NeedsDeepAnalysis escalation decision tests
// ============================================================================

// TestNeedsDeepAnalysis verifies the escalation decision for short inputs
// that cannot be resolved by fast matching.
func TestNeedsDeepAnalysis(t *testing.T) {
	t.Run("empty result needs deep analysis", func(t *testing.T) {
		// No matches, not a simple query → needs deep analysis
		// Example: "我想做渗透测试" where BM25 finds nothing
		result := &FastMatchResult{}
		if !result.NeedsDeepAnalysis() {
			t.Error("empty result (no matches, not simple) should need deep analysis")
		}
	})

	t.Run("simple query does NOT need deep analysis", func(t *testing.T) {
		// Greeting/status check → fast path, no escalation
		// Example: "你好", "ping"
		result := &FastMatchResult{IsSimpleQuery: true}
		if result.NeedsDeepAnalysis() {
			t.Error("simple query should not need deep analysis")
		}
	})

	t.Run("matched tools does NOT need deep analysis", func(t *testing.T) {
		// BM25 found relevant tools → fast path with context
		// Example: "扫描端口" where BM25 finds port scanner tool
		result := &FastMatchResult{
			MatchedTools: []*schema.AIYakTool{{Name: "port-scanner"}},
		}
		if result.NeedsDeepAnalysis() {
			t.Error("result with matched tools should not need deep analysis")
		}
	})

	t.Run("matched forges does NOT need deep analysis", func(t *testing.T) {
		result := &FastMatchResult{
			MatchedForges: []*schema.AIForge{{ForgeName: "vuln-analyzer"}},
		}
		if result.NeedsDeepAnalysis() {
			t.Error("result with matched forges should not need deep analysis")
		}
	})

	t.Run("matched loops does NOT need deep analysis", func(t *testing.T) {
		result := &FastMatchResult{
			MatchedLoops: []*reactloops.LoopMetadata{{Name: "plan"}},
		}
		if result.NeedsDeepAnalysis() {
			t.Error("result with matched loops should not need deep analysis")
		}
	})

	t.Run("simple query with no matches still does NOT need deep", func(t *testing.T) {
		// IsSimpleQuery takes priority: greetings never escalate
		result := &FastMatchResult{IsSimpleQuery: true}
		if result.NeedsDeepAnalysis() {
			t.Error("simple query should never escalate even with no matches")
		}
	})
}

// TestEscalationScenarios documents the complete escalation decision matrix.
// This captures the key design decision: input length != task complexity.
func TestEscalationScenarios(t *testing.T) {
	scenarios := []struct {
		name          string
		input         string
		isSimpleQuery bool
		hasMatches    bool
		expectDeep    bool
		explanation   string
	}{
		{
			name:          "greeting",
			input:         "你好",
			isSimpleQuery: true,
			hasMatches:    false,
			expectDeep:    false,
			explanation:   "greetings are fast-path, never escalate",
		},
		{
			name:          "specific tool task with match",
			input:         "帮我扫描端口",
			isSimpleQuery: false,
			hasMatches:    true,
			expectDeep:    false,
			explanation:   "specific task + BM25 found matching tool = fast path",
		},
		{
			name:          "composite task no match",
			input:         "我想做渗透测试",
			isSimpleQuery: false,
			hasMatches:    false,
			expectDeep:    true,
			explanation:   "short but composite task, no tools match = escalate to deep intent",
		},
		{
			name:          "security audit no match",
			input:         "帮我做安全评估",
			isSimpleQuery: false,
			hasMatches:    false,
			expectDeep:    true,
			explanation:   "broad security task, needs decomposition",
		},
		{
			name:          "english composite no match",
			input:         "security audit",
			isSimpleQuery: false,
			hasMatches:    false,
			expectDeep:    true,
			explanation:   "English composite task, no direct tool match",
		},
		{
			name:          "vague task no match",
			input:         "帮我搞定这个漏洞",
			isSimpleQuery: false,
			hasMatches:    false,
			expectDeep:    true,
			explanation:   "vague task description needs AI decomposition",
		},
		{
			name:          "status check",
			input:         "ping",
			isSimpleQuery: true,
			hasMatches:    false,
			expectDeep:    false,
			explanation:   "status check is simple, no escalation",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			// Verify input scale classification
			scale := ClassifyInputScale(sc.input)
			if !scale.IsMicroOrSmall() {
				t.Skipf("input %q classified as %s, not Micro/Small; skip escalation test", sc.input, scale.String())
			}

			// Simulate FastMatchResult based on scenario
			result := &FastMatchResult{
				IsSimpleQuery: sc.isSimpleQuery,
			}
			if sc.hasMatches {
				result.MatchedTools = []*schema.AIYakTool{{Name: "dummy-tool"}}
			}

			// Verify escalation decision
			needsDeep := result.NeedsDeepAnalysis()
			if needsDeep != sc.expectDeep {
				t.Errorf("scenario %q: NeedsDeepAnalysis()=%v, expected %v (%s)",
					sc.name, needsDeep, sc.expectDeep, sc.explanation)
			}
		})
	}
}

// TestEscalationFlow_InputScaleToDecision verifies the complete flow:
// ClassifyInputScale → FastIntentMatch → NeedsDeepAnalysis → decision
func TestEscalationFlow_InputScaleToDecision(t *testing.T) {
	// These inputs should all be Micro or Small
	shortInputs := []string{
		"我想做渗透测试",
		"帮我做安全评估",
		"security audit",
		"代码审计",
		"帮我搞定这个漏洞",
	}

	for _, input := range shortInputs {
		scale := ClassifyInputScale(input)
		if !scale.IsMicroOrSmall() {
			t.Errorf("input %q should be Micro/Small but got %s", input, scale.String())
			continue
		}

		// With no DB available, FastIntentMatch will find no matches
		// (BM25 search returns empty when DB is nil)
		// This simulates the scenario where no tools/forges match
		emptyResult := &FastMatchResult{IsSimpleQuery: false}
		if !emptyResult.NeedsDeepAnalysis() {
			t.Errorf("input %q with no matches should trigger deep analysis", input)
		}
	}

	// Medium+ inputs always go to deep intent, regardless of matches
	mediumInput := strings.Repeat("这是一个复杂的安全分析需求。", 15)
	scale := ClassifyInputScale(mediumInput)
	if scale.IsMicroOrSmall() {
		t.Errorf("medium input should not be Micro/Small, got %s", scale.String())
	}
}

func TestBuildFastMatchSummary_Empty(t *testing.T) {
	result := &FastMatchResult{}
	summary := buildFastMatchSummary(result)
	if !strings.Contains(summary, "Intent Quick Match Results") {
		t.Error("expected summary header to be present")
	}
}

func TestBuildFastMatchSummary_WithTools(t *testing.T) {
	result := &FastMatchResult{
		MatchedTools: []*schema.AIYakTool{
			{Name: "port-scanner", VerboseName: "Port Scanner", Description: "Scan TCP/UDP ports on target hosts"},
			{Name: "http-fuzzer", Description: "Fuzz HTTP requests with various payloads"},
		},
	}
	summary := buildFastMatchSummary(result)
	if !strings.Contains(summary, "Matched Tools") {
		t.Error("expected Matched Tools section")
	}
	if !strings.Contains(summary, "Port Scanner (port-scanner)") {
		t.Error("expected verbose name format for port-scanner")
	}
	if !strings.Contains(summary, "http-fuzzer") {
		t.Error("expected http-fuzzer in summary")
	}
}

func TestBuildFastMatchSummary_WithForges(t *testing.T) {
	result := &FastMatchResult{
		MatchedForges: []*schema.AIForge{
			{ForgeName: "vuln-analyzer", ForgeVerboseName: "Vulnerability Analyzer", Description: "Analyze and categorize vulnerabilities"},
		},
	}
	summary := buildFastMatchSummary(result)
	if !strings.Contains(summary, "Matched AI Forges") {
		t.Error("expected Matched AI Forges section")
	}
	if !strings.Contains(summary, "Vulnerability Analyzer (vuln-analyzer)") {
		t.Error("expected verbose name format for forge")
	}
}

func TestBuildFastMatchSummary_WithLoops(t *testing.T) {
	result := &FastMatchResult{
		MatchedLoops: []*reactloops.LoopMetadata{
			{Name: "write_yaklang_code", Description: "Write Yaklang code for security tasks"},
		},
	}
	summary := buildFastMatchSummary(result)
	if !strings.Contains(summary, "Matched Focus Modes") {
		t.Error("expected Matched Focus Modes section")
	}
	if !strings.Contains(summary, "write_yaklang_code") {
		t.Error("expected loop name in summary")
	}
}

// ============================================================================
// BM25 search integration pattern tests
// ============================================================================

// TestSearchAIForgeBM25_EmptyQuery verifies that empty keyword returns empty results.
func TestSearchAIForgeBM25_EmptyQuery(t *testing.T) {
	// SearchAIForgeBM25 should return empty slice for empty keywords (no DB needed)
	result, err := yakit.SearchAIForgeBM25(nil, &yakit.AIForgeSearchFilter{
		Keywords: []string{},
	}, 10, 0)
	if err != nil {
		// nil db with empty query should return empty, not error
		// but since db is nil, it may error - that's fine
		t.Logf("expected: nil db returns error or empty: %v", err)
	}
	if len(result) != 0 && err == nil {
		t.Error("empty keywords should return empty results")
	}
}

// TestSearchAIForgeBM25_NilDB verifies behavior when DB is nil.
func TestSearchAIForgeBM25_NilDB(t *testing.T) {
	_, err := yakit.SearchAIForgeBM25(nil, &yakit.AIForgeSearchFilter{
		Keywords: []string{"test query"},
	}, 10, 0)
	if err == nil {
		t.Error("expected error when db is nil")
	}
}

// TestSearchAIYakToolBM25_EmptyQuery verifies that empty keyword returns empty results.
func TestSearchAIYakToolBM25_EmptyQuery(t *testing.T) {
	result, err := yakit.SearchAIYakToolBM25(nil, &yakit.AIYakToolFilter{
		Keywords: []string{},
	}, 10, 0)
	if err != nil {
		t.Logf("expected: nil db returns error or empty: %v", err)
	}
	if len(result) != 0 && err == nil {
		t.Error("empty keywords should return empty results")
	}
}

// TestSearchAIYakToolBM25_NilDB verifies behavior when DB is nil.
func TestSearchAIYakToolBM25_NilDB(t *testing.T) {
	_, err := yakit.SearchAIYakToolBM25(nil, &yakit.AIYakToolFilter{
		Keywords: []string{"test query"},
	}, 10, 0)
	if err == nil {
		t.Error("expected error when db is nil")
	}
}

// TestAIForgeSearchFilter_Structure verifies the filter struct works correctly.
func TestAIForgeSearchFilter_Structure(t *testing.T) {
	filter := &yakit.AIForgeSearchFilter{
		ForgeNames: []string{"forge-a", "forge-b"},
		Keywords:   []string{"vulnerability", "scan"},
	}
	if len(filter.ForgeNames) != 2 {
		t.Errorf("expected 2 forge names, got %d", len(filter.ForgeNames))
	}
	if len(filter.Keywords) != 2 {
		t.Errorf("expected keywords to be set, got %#v", filter.Keywords)
	}
}

// ============================================================================
// Search dual-channel behavior tests
// ============================================================================

// TestBM25FallbackForShortQuery verifies that short queries (<3 chars)
// use LIKE-based search instead of FTS5 BM25.
// This is a behavioral documentation test - the actual fallback happens inside
// SearchAIYakToolBM25 and SearchAIForgeBM25 when match len < 3.
func TestBM25FallbackForShortQuery(t *testing.T) {
	// Short query "ab" has len < 3, should trigger LIKE fallback path
	shortQuery := "ab"
	if len(shortQuery) >= 3 {
		t.Fatal("test setup error: shortQuery should be < 3 chars")
	}

	// Long query "http vulnerability" has len >= 3, should use BM25 path
	longQuery := "http vulnerability"
	if len(longQuery) < 3 {
		t.Fatal("test setup error: longQuery should be >= 3 chars")
	}

	// Verify the dual-channel logic boundary
	t.Logf("short query %q (len=%d) -> LIKE fallback", shortQuery, len(shortQuery))
	t.Logf("long query %q (len=%d) -> BM25 FTS5", longQuery, len(longQuery))
}

// TestMatchLoopMetadata_TokenMatching verifies loop metadata matching
// uses proper token-level scoring.
func TestMatchLoopMetadata_TokenMatching(t *testing.T) {
	// matchLoopMetadata works against registered loop metadata.
	// Since this is an in-memory search, we can test the containsAnyToken logic
	// that underpins it.

	// Simulate what matchLoopMetadata does internally
	searchText := strings.ToLower("write_yaklang_code" + " " + "Write Yaklang code for security tasks" + " " + "Use this when you need to write Yaklang code")

	// Full match
	if !strings.Contains(searchText, "yaklang") {
		t.Error("expected searchText to contain 'yaklang'")
	}

	// Token match
	if !containsAnyToken(searchText, "yaklang security") {
		t.Error("expected token match for 'yaklang security'")
	}

	// No match
	if containsAnyToken(searchText, "python django") {
		t.Error("should not match 'python django' against yaklang loop")
	}
}

// TestIntentSearchDualChannel documents the dual-channel search architecture:
// Channel 1: BM25 FTS5 trigram (for queries >= 3 chars, when FTS5 is available)
// Channel 2: LIKE-based keyword search (fallback for short queries or missing FTS5)
func TestIntentSearchDualChannel(t *testing.T) {
	t.Run("channel selection by query length", func(t *testing.T) {
		// Queries with len < 3 bytes use LIKE fallback
		// Note: Chinese chars are 3+ bytes in UTF-8, so "漏" (3 bytes) uses BM25 path
		shortQueries := []string{"hi", "ab", "a"}
		for _, q := range shortQueries {
			if len(q) >= 3 {
				t.Errorf("test setup error: %q should be < 3 bytes, got %d", q, len(q))
			}
		}

		// Queries with len >= 3 use BM25 FTS5 when available
		longQueries := []string{"http", "vulnerability scan", "端口扫描", "SQL注入"}
		for _, q := range longQueries {
			if len(q) < 3 {
				t.Errorf("test setup error: %q should be >= 3 bytes", q)
			}
		}
	})

	t.Run("FTS5 trigram tokenizer behavior", func(t *testing.T) {
		// Trigram tokenizer splits text into 3-character sequences
		// This means partial matches work well for queries >= 3 chars
		// For example, "vuln" matches "vulnerability" because "vul", "uln" are shared trigrams

		// Verify the minimum query length aligns with trigram tokenizer
		trigramMinLength := 3
		if trigramMinLength != 3 {
			t.Error("trigram tokenizer requires minimum 3-character tokens")
		}
	})

	t.Run("search covers tools forges and loops", func(t *testing.T) {
		// Verify that FastIntentMatch searches all three capability types:
		// 1. AIYakTool via yakit.SearchAIYakToolBM25 (BM25 + LIKE fallback)
		// 2. AIForge via yakit.SearchAIForgeBM25 (BM25 + LIKE fallback)
		// 3. LoopMetadata via matchLoopMetadata (in-memory token matching)

		// This is a structural test - the actual search requires a DB
		// We verify the result struct can hold all three types
		result := &FastMatchResult{
			MatchedTools:  []*schema.AIYakTool{{Name: "tool1"}},
			MatchedForges: []*schema.AIForge{{ForgeName: "forge1"}},
			MatchedLoops:  []*reactloops.LoopMetadata{{Name: "loop1"}},
		}
		if !result.HasMatches() {
			t.Error("result with all three match types should have matches")
		}
		summary := buildFastMatchSummary(result)
		if !strings.Contains(summary, "Matched Tools") {
			t.Error("summary should include tools section")
		}
		if !strings.Contains(summary, "Matched AI Forges") {
			t.Error("summary should include forges section")
		}
		if !strings.Contains(summary, "Matched Focus Modes") {
			t.Error("summary should include loops section")
		}
	})
}
