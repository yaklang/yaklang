package scannode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const testLegionRule = "println(* as $input)\nalert $input"

func testLegionRuleContract(t *testing.T) *legionFocusExecutionContract {
	t.Helper()
	contract := legionFocusExecutionContract{
		SchemaVersion: legionFocusExecutionContractSchemaV1,
		Stages:        []legionFocusExecutionStage{{Key: "generate"}, {Key: "debug"}, {Key: "report"}},
		Capabilities: []string{
			serverFocusCapabilityOriginalSampleRead, serverFocusCapabilityRuleCheck, serverFocusCapabilityRuleDebug, serverFocusCapabilityRuleCandidate,
			serverFocusCapabilitySourceWorkspaceInfo, serverFocusCapabilitySourceList, serverFocusCapabilitySourceRead,
			serverFocusCapabilitySourceSearch, serverFocusCapabilityTaskStage,
		},
		Results: []legionFocusExecutionResultContract{{
			Key: "rule_candidate", Capability: serverFocusCapabilityRuleCandidate,
			Kind: legionSyntaxFlowRuleCandidateKind, Required: true,
		}},
	}
	sort.Strings(contract.Capabilities)
	raw, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseLegionFocusExecutionContract(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func testLegionRuleRuntime(t *testing.T, ctx context.Context, files map[string]string) (*legionServerFocusRuntime, *recordingAIFocusRiskPublisher) {
	t.Helper()
	if !legionSyntaxFlowRuntimeAvailable() {
		t.Skip("this build excludes the full SyntaxFlow language runtime")
	}
	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindInlineSources)
	spec.Locator = ""
	spec.InlineFiles = files
	workspace, err := materializeLegionInlineWorkspace(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	publisher := &recordingAIFocusRiskPublisher{}
	sink, err := newLegionAIFocusResultSink(publisher, "bind-rule-candidate", validCodeAuditResultContext())
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.(aiFocusCodeWorkspaceEvidenceBinder).bindCodeWorkspaceEvidence(workspace.lockedRevision, workspace.sha256); err != nil {
		t.Fatal(err)
	}
	target, err := legionCodeWorkspaceSentinel(workspace.spec.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	value, err := newLegionServerFocusRuntime(ctx, target, sink, workspace)
	if err != nil {
		t.Fatal(err)
	}
	runtime := value.(*legionServerFocusRuntime)
	runtime.authorizedFocusReleaseID = "syntaxflow_rule@1.0.0+abcdef123456"
	if err := runtime.activateFocusTurn(runtime.authorizedFocusReleaseID, testLegionRuleContract(t)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runtime.deactivateFocusTurn(runtime.authorizedFocusReleaseID) })
	return runtime, publisher
}

func testLegionRuleCandidateParams() map[string]any {
	return map[string]any{
		"title": "SyntaxFlow 规则候选", "rule_name": "bounded-print.sf", "language": "yak",
		"rule": testLegionRule, "explanation": "检查 println 的参数。",
	}
}

func TestLegionSyntaxFlowOriginalSampleReadUsesPrivateBoundBytes(t *testing.T) {
	root := t.TempDir()
	path := "negative/Sample.java"
	if err := os.MkdirAll(filepath.Join(root, "negative"), 0o750); err != nil {
		t.Fatal(err)
	}
	projectContent := `class ProjectCollision {}`
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(projectContent), 0o640); err != nil {
		t.Fatal(err)
	}
	trustedContent := `class Negative { void run() throws Exception { Runtime.getRuntime().exec("echo safe"); } }`
	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
	spec.InlineFiles = map[string]string{path: trustedContent}
	spec.SyntaxFlowMode = "improve"
	spec.SyntaxFlowLanguage = "java"
	spec.SyntaxFlowOriginalRule = "exec(,* as $cmd,)\nalert $cmd"
	spec.SyntaxFlowOriginalRuleSHA256 = legionRuleHash(spec.SyntaxFlowOriginalRule)
	spec.SyntaxFlowOriginalSamplePath = path
	spec.SyntaxFlowOriginalSampleSHA256 = legionInlineSourceDigest(spec.InlineFiles)
	spec.SyntaxFlowRequireOriginalReproduction = true
	if err := normalizeLegionCodeWorkspaceSpec(&spec); err != nil {
		t.Fatal(err)
	}
	workspace := &legionCodeWorkspaceRuntime{
		spec:           publicLegionCodeWorkspaceSpec(spec),
		root:           root,
		lockedRevision: strings.Repeat("a", 40),
		sha256:         strings.Repeat("b", 64),
		inlineFiles:    cloneLegionInlineFiles(spec.InlineFiles),
		originalRule:   spec.SyntaxFlowOriginalRule,
	}
	target, err := legionCodeWorkspaceSentinel(spec.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	value, err := newLegionServerFocusRuntime(context.Background(), target, &recordingServerFocusSink{}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	runtime := value.(*legionServerFocusRuntime)
	runtime.authorizedFocusReleaseID = "syntaxflow_rule@1.0.0+abcdef123456"
	if err := runtime.activateFocusTurn(runtime.authorizedFocusReleaseID, testLegionRuleContract(t)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runtime.deactivateFocusTurn(runtime.authorizedFocusReleaseID) })

	read, err := runtime.Execute(serverFocusCapabilityOriginalSampleRead, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if read["path"] != path || read["content"] != trustedContent || read["content"] == projectContent ||
		read["offset"] != 0 || read["read_bytes"] != len(trustedContent) || read["file_size"] != len(trustedContent) || read["truncated"] != false ||
		read["source_kind"] != "inline_sample" || read["sample_origin"] != "user_supplied" || read["source_sha256"] != spec.SyntaxFlowOriginalSampleSHA256 {
		t.Fatalf("private original sample read lost its exact pin: %#v", read)
	}
	if _, err := runtime.Execute(serverFocusCapabilityOriginalSampleRead, map[string]any{"path": path}); err == nil {
		t.Fatal("model-controlled original sample path was accepted")
	}
	workspace.inlineFiles[path] = `class Tampered {}`
	if _, err := runtime.Execute(serverFocusCapabilityOriginalSampleRead, map[string]any{}); err == nil || !strings.Contains(err.Error(), "server pin") {
		t.Fatalf("mutated private original sample failed open: %v", err)
	}
}

func testLegionRuleDebug(t *testing.T, runtime *legionServerFocusRuntime, params map[string]any) legionSyntaxFlowDebugResult {
	t.Helper()
	value, err := runtime.Execute(serverFocusCapabilityRuleDebug, params)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(value)
	var result legionSyntaxFlowDebugResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func testLegionLastCandidate(t *testing.T, publisher *recordingAIFocusRiskPublisher) (aiFocusCodeAuditReport, legionSyntaxFlowRuleCandidateSummary) {
	t.Helper()
	if len(publisher.reports) == 0 {
		t.Fatal("candidate report was not published")
	}
	var report aiFocusCodeAuditReport
	if err := json.Unmarshal(publisher.reports[len(publisher.reports)-1], &report); err != nil {
		t.Fatal(err)
	}
	var summary legionSyntaxFlowRuleCandidateSummary
	if err := json.Unmarshal(report.StructuredSummary, &summary); err != nil {
		t.Fatal(err)
	}
	return report, summary
}

func TestLegionSyntaxFlowCheckAndAuthority(t *testing.T) {
	runtime, _ := testLegionRuleRuntime(t, context.Background(), nil)
	valid, err := runtime.Execute(serverFocusCapabilityRuleCheck, map[string]any{"rule": testLegionRule})
	if err != nil || valid["status"] != "valid" || valid["rule_sha256"] != legionRuleHash(testLegionRule) {
		t.Fatalf("valid grammar: %#v err=%v", valid, err)
	}
	for _, rule := range []string{"println((", "<include(\"unbound-library\")>", "<eval(\"println(*)\")>", "<fuzztag(\"{{file(/etc/passwd)}}\")>"} {
		result, err := runtime.Execute(serverFocusCapabilityRuleCheck, map[string]any{"rule": rule})
		if err != nil || result["status"] != "invalid" {
			t.Fatalf("unsafe/invalid grammar accepted: %q result=%#v err=%v", rule, result, err)
		}
	}
	if _, err := runtime.Execute(serverFocusCapabilityRuleCheck, map[string]any{"rule": strings.Repeat("a", legionSyntaxFlowMaxRuleBytes+1)}); err == nil {
		t.Fatal("oversized rule accepted")
	}
	runtime.activeExecutionContract = testLegionCodeWorkspaceExecutionContract(t)
	if _, err := runtime.Execute(serverFocusCapabilityRuleCheck, map[string]any{"rule": testLegionRule}); err == nil {
		t.Fatal("missing immutable capability accepted")
	}
	runtime.deactivateFocusTurn(runtime.authorizedFocusReleaseID)
	for _, capability := range []string{serverFocusCapabilityRuleCheck, serverFocusCapabilityRuleDebug, serverFocusCapabilityRuleCandidate} {
		if _, err := runtime.Execute(capability, map[string]any{"rule": testLegionRule}); err == nil {
			t.Fatalf("dormant Focus exposed %s", capability)
		}
	}
	ordinary := &legionServerFocusRuntime{ctx: context.Background()}
	if _, err := ordinary.Execute(serverFocusCapabilityRuleCheck, map[string]any{"rule": testLegionRule}); err == nil {
		t.Fatal("ordinary non-Focus runtime exposed rule check")
	}
}

func TestLegionSyntaxFlowRealDebugAndCanonicalCandidate(t *testing.T) {
	files := map[string]string{"src/main.yak": "println(\"unsafe\")"}
	runtime, publisher := testLegionRuleRuntime(t, context.Background(), files)
	result := testLegionRuleDebug(t, runtime, map[string]any{
		"rule": testLegionRule, "language": "yak", "source_kind": "workspace",
		"paths": []string{"src/main.yak"}, "expected": "match",
	})
	if result.Status != "completed" || result.MatchCount != 1 || len(result.Matches) != 1 {
		t.Fatalf("real positive SSA query failed: %#v", result)
	}
	if result.SourceKind != "workspace_files" || result.WorkspaceRevision != runtime.workspace.lockedRevision ||
		result.SourceSHA256 != legionInlineSourceDigest(files) || result.Matches[0].Path != "src/main.yak" || result.Matches[0].StartLine != 1 {
		t.Fatalf("workspace provenance/locations are incorrect: %#v", result)
	}
	noMatch := testLegionRuleDebug(t, runtime, map[string]any{
		"rule": testLegionRule, "language": "yak", "source_kind": "inline",
		"files": map[string]string{"safe.yak": "other(\"safe\")"}, "expected": "no_match",
	})
	if noMatch.Status != "completed" || noMatch.MatchCount != 0 || noMatch.SourceKind != "inline_sample" ||
		noMatch.SampleOrigin != "generated_or_modified" || noMatch.WorkspaceRevision != "" {
		t.Fatalf("real negative SSA query/provenance failed: %#v", noMatch)
	}
	baseline := "not_present as $baseline\nalert $baseline"
	baselineResult := testLegionRuleDebug(t, runtime, map[string]any{
		"rule": baseline, "language": "yak", "source_kind": "inline", "files": files,
	})
	if baselineResult.Status != "completed" || baselineResult.SampleOrigin != "user_supplied" || baselineResult.RuleSHA256 == result.RuleSHA256 {
		t.Fatalf("baseline observations lost: %#v", baselineResult)
	}
	forged := testLegionRuleCandidateParams()
	forged["debug_results"] = []map[string]any{{"status": "completed", "match_count": 999}}
	if _, err := runtime.Execute(serverFocusCapabilityRuleCandidate, forged); err == nil {
		t.Fatal("model-supplied evidence accepted")
	}
	params := testLegionRuleCandidateParams()
	params["markdown"] = "FORGED verified all project tests passed"
	if _, err := runtime.Execute(serverFocusCapabilityRuleCandidate, params); err != nil {
		t.Fatal(err)
	}
	report, summary := testLegionLastCandidate(t, publisher)
	if publisher.reportKinds[0] != legionSyntaxFlowRuleCandidateKind || len(publisher.risks) != 0 {
		t.Fatalf("candidate wrote the wrong transport: %#v", publisher)
	}
	if summary.RuleContent != testLegionRule || summary.RuleSHA256 != legionRuleHash(testLegionRule) ||
		summary.VerificationStatus != "tested" || len(summary.DebugResults) != 3 ||
		summary.DebugResults[2].RuleSHA256 != legionRuleHash(baseline) || strings.Contains(report.Markdown, "FORGED") {
		t.Fatalf("candidate did not use canonical runtime evidence: %#v", summary)
	}
	if !strings.Contains(report.Markdown, "不代表原项目") || !strings.Contains(report.Markdown, "不表示回归通过") {
		t.Fatalf("report overclaims debug evidence: %s", report.Markdown)
	}
	if err := runtime.sink.(*legionAIFocusResultSink).Succeed(context.Background(), nil); err != nil {
		t.Fatalf("candidate did not satisfy required report contract: %v", err)
	}
}

func TestLegionSyntaxFlowJavaCallArgumentOpcodeFilter(t *testing.T) {
	files := map[string]string{
		"positive/Sample.java": `class Positive { void run(String command) throws Exception { Runtime.getRuntime().exec(command); } }`,
		"negative/Sample.java": `class Negative { void run() throws Exception { Runtime.getRuntime().exec("echo safe"); } }`,
	}
	runtime, _ := testLegionRuleRuntime(t, context.Background(), files)
	const rule = "exec(,*?{!opcode: const} as $cmd,)\nalert $cmd"
	positive := testLegionRuleDebug(t, runtime, map[string]any{
		"rule": rule, "language": "java", "source_kind": "workspace",
		"paths": []string{"positive/Sample.java"}, "expected": "match", "label": "positive dynamic command",
	})
	negative := testLegionRuleDebug(t, runtime, map[string]any{
		"rule": rule, "language": "java", "source_kind": "workspace",
		"paths": []string{"negative/Sample.java"}, "expected": "no_match", "label": "negative literal command",
	})
	if positive.Status != "completed" || positive.MatchCount == 0 {
		t.Fatalf("non-constant Java call argument was not matched: %#v", positive)
	}
	if negative.Status != "completed" || negative.MatchCount != 0 {
		t.Fatalf("constant Java call argument was not excluded: %#v", negative)
	}
}

func TestLegionSyntaxFlowCannotReuseTurnOrRuleEvidence(t *testing.T) {
	runtime, publisher := testLegionRuleRuntime(t, context.Background(), nil)
	input := map[string]any{
		"rule": testLegionRule, "language": "yak", "source_kind": "inline",
		"files": map[string]string{"main.yak": "println(1)"},
	}
	value, err := runtime.Execute(serverFocusCapabilityRuleDebug, input)
	if err != nil || value["status"] != "completed" {
		t.Fatalf("initial debug: %#v %v", value, err)
	}
	// Mutating the returned map must not change retained runtime observations.
	value["match_count"] = 999
	value["rule_sha256"] = strings.Repeat("f", 64)
	params := testLegionRuleCandidateParams()
	params["rule"] = testLegionRule + "\n// changed candidate"
	if _, err := runtime.Execute(serverFocusCapabilityRuleCandidate, params); err != nil {
		t.Fatal(err)
	}
	_, different := testLegionLastCandidate(t, publisher)
	if different.VerificationStatus != "syntax_only" || different.DebugResults[0].MatchCount == 999 {
		t.Fatalf("different rule or mutated evidence was reused: %#v", different)
	}
	params = testLegionRuleCandidateParams()
	params["language"] = "java"
	if _, err := runtime.Execute(serverFocusCapabilityRuleCandidate, params); err != nil {
		t.Fatal(err)
	}
	_, differentLanguage := testLegionLastCandidate(t, publisher)
	if differentLanguage.VerificationStatus != "syntax_only" {
		t.Fatalf("debug in another language was reused: %#v", differentLanguage)
	}
	release := runtime.authorizedFocusReleaseID
	runtime.deactivateFocusTurn(release)
	if err := runtime.activateFocusTurn(release, testLegionRuleContract(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Execute(serverFocusCapabilityRuleCandidate, testLegionRuleCandidateParams()); err != nil {
		t.Fatal(err)
	}
	_, nextTurn := testLegionLastCandidate(t, publisher)
	if nextTurn.VerificationStatus != "syntax_only" || len(nextTurn.DebugResults) != 0 {
		t.Fatalf("previous Turn evidence survived activation: %#v", nextTurn)
	}
}

func TestLegionSyntaxFlowSourceSelectionBoundary(t *testing.T) {
	runtime, _ := testLegionRuleRuntime(t, context.Background(), map[string]string{"main.yak": "println(1)"})
	targets := []map[string]any{
		{"source_kind": "workspace"},
		{"source_kind": "workspace", "paths": []string{"../outside.yak"}},
		{"source_kind": "workspace", "paths": []string{"/etc/passwd"}},
		{"source_kind": "workspace", "paths": []string{"C:/secret.yak"}},
		{"source_kind": "workspace", "paths": []string{"main.yak", "main.yak"}},
		{"source_kind": "workspace", "paths": []string{"main.yak"}, "files": map[string]string{}},
		{"source_kind": "inline", "files": map[string]string{}, "paths": []string{"main.yak"}},
		{"source_kind": "inline", "files": map[string]string{"..\\outside.yak": "1"}},
		{"source_kind": "inline", "files": map[string]string{"a.yak": strings.Repeat("x", legionInlineSourceMaxFileBytes+1)}},
		{"source_kind": "inline", "files": map[string]any{"a.yak": 1}},
		{"source_kind": "inline", "files": map[string]string{"a.yak": "1"}, "program_name": "another-user-program"},
		{"source_kind": "inline", "files": map[string]string{"a.yak": "1"}, "sample_origin": "user_supplied"},
	}
	for i, params := range targets {
		params["rule"], params["language"] = testLegionRule, "yak"
		if _, err := runtime.Execute(serverFocusCapabilityRuleDebug, params); err == nil {
			t.Fatalf("unsafe target %d accepted: %#v", i, params)
		}
	}
	link := filepath.Join(runtime.workspace.root, "link.yak")
	if err := os.Symlink("main.yak", link); err == nil {
		if _, err := runtime.Execute(serverFocusCapabilityRuleDebug, map[string]any{
			"rule": testLegionRule, "language": "yak", "source_kind": "workspace", "paths": []string{"link.yak"},
		}); err == nil {
			t.Fatal("in-workspace symlink accepted")
		}
	}
	if len(runtime.ruleDebugHistory) != 0 {
		t.Fatal("invalid source selections created debug evidence")
	}
}

func TestLegionSyntaxFlowCancellationTimeoutBudgetAndFailure(t *testing.T) {
	input := func() map[string]any {
		return map[string]any{
			"rule": testLegionRule, "language": "yak", "source_kind": "inline",
			"files": map[string]string{"main.yak": "a=1\nb=2\nc=3\nprintln(a+b+c)"},
		}
	}
	t.Run("timeout", func(t *testing.T) {
		runtime, publisher := testLegionRuleRuntime(t, context.Background(), nil)
		runtime.ruleDebugTimeout = time.Nanosecond
		result := testLegionRuleDebug(t, runtime, input())
		if result.Status != "timeout" {
			t.Fatalf("timeout classified as success: %#v", result)
		}
		if _, err := runtime.Execute(serverFocusCapabilityRuleCandidate, testLegionRuleCandidateParams()); err != nil {
			t.Fatal(err)
		}
		_, candidate := testLegionLastCandidate(t, publisher)
		if candidate.VerificationStatus != "debug_failed" {
			t.Fatalf("timed-out run became tested: %#v", candidate)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		runtime, _ := testLegionRuleRuntime(t, ctx, nil)
		cancel()
		result := testLegionRuleDebug(t, runtime, input())
		if result.Status != "cancelled" {
			t.Fatalf("cancellation classified as success: %#v", result)
		}
	})
	t.Run("work-budget", func(t *testing.T) {
		runtime, _ := testLegionRuleRuntime(t, context.Background(), nil)
		runtime.ruleWorkLimit = 1
		params := input()
		params["rule"] = "println(* as $input)\n$input<typeName> as $types\nalert $types"
		params["files"] = map[string]string{"main.yak": "println(1,2,3)"}
		result := testLegionRuleDebug(t, runtime, params)
		if result.Status != "work_budget_exceeded" {
			t.Fatalf("real fanout budget not enforced: %#v", result)
		}
	})
	t.Run("invalid-rule", func(t *testing.T) {
		runtime, _ := testLegionRuleRuntime(t, context.Background(), nil)
		params := input()
		params["rule"] = "println(("
		result := testLegionRuleDebug(t, runtime, params)
		if result.Status != "invalid_rule" {
			t.Fatalf("invalid rule classification: %#v", result)
		}
	})
	t.Run("compile-error", func(t *testing.T) {
		runtime, _ := testLegionRuleRuntime(t, context.Background(), nil)
		params := input()
		params["files"] = map[string]string{"main.yak": "println(\""}
		result := testLegionRuleDebug(t, runtime, params)
		if result.Status != "compile_error" {
			t.Fatalf("invalid source classification: %#v", result)
		}
	})
	t.Run("call-budget", func(t *testing.T) {
		runtime, _ := testLegionRuleRuntime(t, context.Background(), nil)
		for i := 0; i < legionSyntaxFlowMaxCalls; i++ {
			if _, err := runtime.Execute(serverFocusCapabilityRuleCheck, map[string]any{"rule": testLegionRule}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := runtime.Execute(serverFocusCapabilityRuleDebug, input()); err == nil {
			t.Fatal("per-Turn call budget was not enforced")
		}
	})
}

func TestLegionSyntaxFlowCandidateSinkRejectsForgedReport(t *testing.T) {
	runtime, _ := testLegionRuleRuntime(t, context.Background(), nil)
	sink := runtime.sink.(aiFocusCodeResultSink)
	if _, err := sink.SubmitCodeAuditReport(context.Background(), legionSyntaxFlowRuleCandidateKind, aiFocusCodeAuditReport{
		WorkspaceID: runtime.workspace.spec.WorkspaceID, Title: "forged", Markdown: "passed",
		StructuredSummary: json.RawMessage(`{"schema_version":"legion.syntaxflow-rule-candidate/v1","verification_status":"tested"}`),
	}); err == nil {
		t.Fatal("raw report bypassed dedicated candidate evidence primitive")
	}
	contract := testLegionRuleContract(t)
	for _, change := range []func(*legionFocusExecutionContract){
		func(c *legionFocusExecutionContract) {
			c.Results[0].Capability = "result.report.v1"
			c.Capabilities = append(c.Capabilities, "result.report.v1")
		},
		func(c *legionFocusExecutionContract) { c.Results[0].Kind = "ai_code_audit_v1" },
		func(c *legionFocusExecutionContract) { c.Results = nil },
	} {
		candidate := cloneLegionFocusExecutionContract(contract)
		change(candidate)
		sort.Strings(candidate.Capabilities)
		raw, _ := json.Marshal(candidate)
		if _, err := parseLegionFocusExecutionContract(string(raw)); err == nil {
			t.Fatalf("candidate result transport substitution accepted: %s", raw)
		}
	}
}

func TestLegionSyntaxFlowCandidateCapsEvidenceWithoutChangingRule(t *testing.T) {
	records := make([]legionSyntaxFlowDebugResult, legionSyntaxFlowMaxCalls)
	for i := range records {
		records[i] = legionSyntaxFlowDebugResult{
			SchemaVersion: legionSyntaxFlowDebugSchema, DebugID: strings.Repeat("a", 36),
			RuleSHA256: legionRuleHash(testLegionRule), SourceSHA256: strings.Repeat("b", 64),
			SourceKind: "inline_sample", SampleOrigin: "generated_or_modified", Language: "yak",
			Status: "completed", MatchCount: 32, Matches: make([]legionSyntaxFlowMatch, 32),
			Diagnostics: []string{}, Expected: "observe",
		}
		for j := range records[i].Matches {
			records[i].Matches[j] = legionSyntaxFlowMatch{Code: strings.Repeat("x", legionSyntaxFlowMaxSnippetBytes), Variable: "input", Path: "main.yak", StartLine: 1, EndLine: 1}
		}
	}
	summary := legionSyntaxFlowRuleCandidateSummary{
		RuleContent: testLegionRule, RuleSHA256: legionRuleHash(testLegionRule), DebugResults: records,
	}
	raw, err := marshalBoundedLegionRuleCandidate(&summary)
	if err != nil || len(raw) > maxInlineCodeAuditSummaryBytes || summary.RuleContent != testLegionRule || len(summary.DebugResults) != legionSyntaxFlowMaxCalls {
		t.Fatalf("bounded summary changed source evidence identities: size=%d err=%v", len(raw), err)
	}
	wasTrimmed := false
	for _, result := range summary.DebugResults {
		wasTrimmed = wasTrimmed || result.Truncated
		if result.MatchCount != 32 {
			t.Fatal("trimming altered observed match count")
		}
	}
	if !wasTrimmed {
		t.Fatal("expected capped evidence")
	}
}

func TestLegionSyntaxFlowLanguageAliases(t *testing.T) {
	for alias, canonical := range map[string]string{"go": "golang", "golang": "golang", "javascript": "js", "js": "js", "typescript": "ts", "py": "python", "yaklang": "yak"} {
		language, err := normalizeLegionSyntaxFlowLanguage(alias)
		if err != nil || language.String() != canonical {
			t.Fatalf("alias %q: %q %v", alias, language, err)
		}
	}
	for _, invalid := range []string{"", "ruby", "general", "host:/tmp"} {
		if _, err := normalizeLegionSyntaxFlowLanguage(invalid); err == nil {
			t.Fatalf("unavailable language accepted: %s", invalid)
		}
	}
}

func TestLegionSyntaxFlowCandidateRequiresCompiledAlert(t *testing.T) {
	runtime, publisher := testLegionRuleRuntime(t, context.Background(), nil)
	for _, rule := range []string{
		"println(* as $input)",
		"println(* as $input)\n// alert $input",
	} {
		checked, err := runtime.Execute(serverFocusCapabilityRuleCheck, map[string]any{"rule": rule})
		if err != nil || checked["status"] != "valid" {
			t.Fatalf("intermediate query must remain checkable: %#v %v", checked, err)
		}
		debugged := testLegionRuleDebug(t, runtime, map[string]any{
			"rule": rule, "language": "yak", "source_kind": "inline", "files": map[string]string{"main.yak": "println(1)"},
		})
		if debugged.Status != "completed" || debugged.MatchCount != 0 {
			t.Fatalf("generic variable matches must not count as alert evidence: %#v", debugged)
		}
		params := testLegionRuleCandidateParams()
		params["rule"] = rule
		if _, err := runtime.Execute(serverFocusCapabilityRuleCandidate, params); err == nil || !strings.Contains(err.Error(), "compiled alert") {
			t.Fatalf("non-detecting candidate accepted: %v", err)
		}
	}
	if len(publisher.reports) != 0 {
		t.Fatal("intermediate query was persisted as a candidate")
	}
}

func TestLegionSyntaxFlowRealLanguageSupport(t *testing.T) {
	for _, sample := range []struct{ language, path, source string }{
		{"yak", "main.yak", "sink(1)"},
		{"js", "main.js", "sink(1);"},
		{"ts", "main.ts", "function sink(x: number) {}\nsink(1);"},
		{"golang", "main.go", "package main\nfunc sink(x int) {}\nfunc main() { sink(1) }"},
		{"java", "Main.java", "class Main { void run() { sink(1); } void sink(int x) {} }"},
		{"php", "main.php", "<?php function sink($x) {} sink(1);"},
		{"python", "main.py", "def sink(x):\n    pass\nsink(1)\n"},
		{"c", "main.c", "void sink(int x) {}\nint main() { sink(1); return 0; }"},
	} {
		t.Run(sample.language, func(t *testing.T) {
			runtime, _ := testLegionRuleRuntime(t, context.Background(), nil)
			result := testLegionRuleDebug(t, runtime, map[string]any{
				"rule": "sink(* as $input)\nalert $input", "language": sample.language,
				"source_kind": "inline", "files": map[string]string{sample.path: sample.source},
			})
			if result.Status != "completed" || result.MatchCount < 1 {
				t.Fatalf("advertised language failed real compile/query: %#v", result)
			}
		})
	}
}

func TestLegionSyntaxFlowNestedNativeCannotEscape(t *testing.T) {
	runtime, _ := testLegionRuleRuntime(t, context.Background(), nil)
	for _, native := range []string{"include(\"not-authorized\")", "eval(\"println(*)\")", "fuzztag(\"{{int(1)}}\")", "unknownTaskNative"} {
		rule := "println(* as $input)\n$input<dataflow(<<<CODE\n*<" + native + ">\nCODE)>\nalert $input"
		// Only the outer native is present in the compiled frame. The inner
		// frame must inherit the authority guard when it is compiled at runtime.
		if _, err := compileLegionSyntaxFlowRule(rule); err != nil {
			t.Fatalf("test must reach the nested runtime guard: %v", err)
		}
		result := testLegionRuleDebug(t, runtime, map[string]any{
			"rule": rule, "language": "yak", "source_kind": "inline", "files": map[string]string{"main.yak": "println(1)"},
		})
		if result.Status != "query_error" || !strings.Contains(strings.Join(result.Diagnostics, " "), "unsupported") {
			t.Fatalf("nested native bypassed task authority: %s: %#v", native, result)
		}
	}
}

func TestLegionSyntaxFlowLeavesOrdinaryQueryAndCacheUnchanged(t *testing.T) {
	previous, wasCached := ssaapi.ProgramCache.Get("")
	defer func() {
		if wasCached {
			ssaapi.SetProgramCache(previous)
		} else {
			ssaapi.ProgramCache.Remove("")
		}
	}()
	ordinaryQuery := func() {
		fs := filesys.NewVirtualFs()
		fs.AddFile("ordinary.yak", "println(1)")
		programs, err := ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaconfig.Yak), ssaapi.WithMemory(true))
		if err != nil || len(programs) != 1 {
			t.Fatalf("ordinary project compile: %v", err)
		}
		program := programs[0]
		defer program.Program.Cache.CloseWithoutSave()
		if cached, ok := ssaapi.ProgramCache.Get(""); !ok || cached != program {
			t.Fatal("ordinary project compile lost its existing global cache behavior")
		}
		result, err := program.SyntaxFlowWithError("<eval(<<<CODE\nprintln(* as $input)\nalert $input\nCODE)>")
		if err != nil || result == nil || len(result.GetValues("input")) == 0 {
			t.Fatalf("ordinary unrestricted native query changed: %v", err)
		}
	}
	ordinaryQuery()
	cachedBefore, presentBefore := ssaapi.ProgramCache.Get("")
	runtime, _ := testLegionRuleRuntime(t, context.Background(), nil)
	result := testLegionRuleDebug(t, runtime, map[string]any{
		"rule": testLegionRule, "language": "yak", "source_kind": "inline", "files": map[string]string{"main.yak": "println(2)"},
	})
	if result.Status != "completed" || result.MatchCount != 1 {
		t.Fatalf("task query failed: %#v", result)
	}
	cachedAfter, presentAfter := ssaapi.ProgramCache.Get("")
	if presentBefore != presentAfter || cachedBefore != cachedAfter {
		t.Fatal("anonymous task-private program escaped into the global program cache")
	}
	ordinaryQuery()
}

func TestLegionSyntaxFlowDoesNotPersistProgramsRulesOrRisks(t *testing.T) {
	// Initialize the process's test database before measuring. The focused
	// command supplies a task-owned YAKIT_HOME; no global DB is rebound.
	database := ssadb.GetDB()
	profile := consts.GetGormProfileDatabase()
	snapshot := func() map[string]int64 {
		t.Helper()
		counts := map[string]int64{}
		for name, model := range map[string]any{
			"programs": &ssadb.IrProgram{}, "instructions": &ssadb.IrCode{},
			"sources": &ssadb.IrSource{}, "results": &ssadb.AuditResult{}, "risks": &schema.SSARisk{},
		} {
			var count int64
			if database.HasTable(model) {
				if err := database.Model(model).Unscoped().Count(&count).Error; err != nil {
					t.Fatal(err)
				}
			}
			counts[name] = count
		}
		var rules int64
		if profile.HasTable(&schema.SyntaxFlowRule{}) {
			if err := profile.Model(&schema.SyntaxFlowRule{}).Unscoped().Count(&rules).Error; err != nil {
				t.Fatal(err)
			}
		}
		counts["rules"] = rules
		return counts
	}
	before := snapshot()
	runtime, _ := testLegionRuleRuntime(t, context.Background(), nil)
	if _, err := runtime.Execute(serverFocusCapabilityRuleCheck, map[string]any{"rule": testLegionRule}); err != nil {
		t.Fatal(err)
	}
	result := testLegionRuleDebug(t, runtime, map[string]any{
		"rule": testLegionRule, "language": "yak", "source_kind": "inline", "files": map[string]string{"main.yak": "println(3)"},
	})
	if result.Status != "completed" || result.MatchCount != 1 {
		t.Fatalf("real debug failed: %#v", result)
	}
	if _, err := runtime.Execute(serverFocusCapabilityRuleCandidate, testLegionRuleCandidateParams()); err != nil {
		t.Fatal(err)
	}
	for kind, after := range snapshot() {
		if after != before[kind] {
			t.Fatalf("task-private rule execution persisted %s: before=%d after=%d", kind, before[kind], after)
		}
	}
}

func TestResilienceLegionSyntaxFlowTurnCancellationFencesWaitingWorker(t *testing.T) {
	// Exhaust worker slots so the Turn switch deterministically happens
	// before this query can compile. Release only these task-owned slots.
	for i := 0; i < cap(legionSyntaxFlowWorkers); i++ {
		select {
		case legionSyntaxFlowWorkers <- struct{}{}:
		case <-time.After(5 * time.Second):
			t.Fatal("previous bounded debug workers did not finish")
		}
	}
	defer func() {
		for i := 0; i < cap(legionSyntaxFlowWorkers); i++ {
			<-legionSyntaxFlowWorkers
		}
	}()
	runtime, publisher := testLegionRuleRuntime(t, context.Background(), nil)
	done := make(chan error, 1)
	go func() {
		_, err := runtime.Execute(serverFocusCapabilityRuleDebug, map[string]any{
			"rule": testLegionRule, "language": "yak", "source_kind": "inline", "files": map[string]string{"main.yak": "println(1)"},
		})
		done <- err
	}()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
waiting:
	for {
		runtime.mu.Lock()
		started := runtime.ruleCallCount > 0
		runtime.mu.Unlock()
		if started {
			break waiting
		}
		select {
		case <-deadline:
			t.Fatal("debug operation did not start")
		case <-ticker.C:
		}
	}
	release := runtime.authorizedFocusReleaseID
	runtime.deactivateFocusTurn(release)
	if err := runtime.activateFocusTurn(release, testLegionRuleContract(t)); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("old Turn produced observations after deactivation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Turn cancellation did not release the blocked operation")
	}
	if _, err := runtime.Execute(serverFocusCapabilityRuleCandidate, testLegionRuleCandidateParams()); err != nil {
		t.Fatal(err)
	}
	_, candidate := testLegionLastCandidate(t, publisher)
	if candidate.VerificationStatus != "syntax_only" || len(candidate.DebugResults) != 0 {
		t.Fatalf("old worker polluted the next Turn: %#v", candidate)
	}
}
