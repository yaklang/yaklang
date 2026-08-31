package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const (
	serverFocusCapabilityRuleCheck     = "syntaxflow.rule.check"
	serverFocusCapabilityRuleDebug     = "syntaxflow.rule.debug"
	serverFocusCapabilityRuleCandidate = "result.rule_candidate.v1"
	legionSyntaxFlowRuleCandidateKind  = "ai_syntaxflow_rule_v1"
	legionSyntaxFlowCheckSchema        = "legion.syntaxflow-check/v1"
	legionSyntaxFlowDebugSchema        = "legion.syntaxflow-debug/v1"
	legionSyntaxFlowCandidateSchema    = "legion.syntaxflow-rule-candidate/v1"
	legionSyntaxFlowMaxRuleBytes       = 32 * 1024
	legionSyntaxFlowMaxCalls           = 24
	legionSyntaxFlowMaxPaths           = 32
	legionSyntaxFlowMaxMatches         = 32
	legionSyntaxFlowMaxSnippetBytes    = 512
	legionSyntaxFlowMaxDiagnostics     = 8
	legionSyntaxFlowMaxDiagnosticBytes = 512
	legionSyntaxFlowDebugTimeout       = 20 * time.Second
	legionSyntaxFlowWorkBudget         = int64(100_000)
)

// Keep cancelled compiles bounded even while a language parser finishes its
// current (cooperative) step. The worker retains its slot until cleanup ends.
var legionSyntaxFlowWorkers = make(chan struct{}, 2)

type legionSyntaxFlowMatch struct {
	Variable  string `json:"variable"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Code      string `json:"code"`
}

type legionSyntaxFlowDebugResult struct {
	SchemaVersion     string                  `json:"schema_version"`
	DebugID           string                  `json:"debug_id"`
	RuleSHA256        string                  `json:"rule_sha256"`
	SourceKind        string                  `json:"source_kind"`
	SampleOrigin      string                  `json:"sample_origin,omitempty"`
	SourceSHA256      string                  `json:"source_sha256"`
	WorkspaceRevision string                  `json:"workspace_revision"`
	Language          string                  `json:"language"`
	Label             string                  `json:"label"`
	Expected          string                  `json:"expected"`
	Status            string                  `json:"status"`
	MatchCount        int                     `json:"match_count"`
	Matches           []legionSyntaxFlowMatch `json:"matches"`
	Truncated         bool                    `json:"truncated"`
	Diagnostics       []string                `json:"diagnostics"`
}

func legionSyntaxFlowRuntimeAvailable() bool {
	for _, language := range []ssaconfig.Language{ssaconfig.JAVA, ssaconfig.GO, ssaconfig.PHP, ssaconfig.JS, ssaconfig.TS, ssaconfig.PYTHON, ssaconfig.C, ssaconfig.Yak} {
		if ssaapi.LanguageBuilderCreater[language] == nil {
			return false
		}
	}
	return true
}

func normalizeLegionSyntaxFlowLanguage(raw string) (ssaconfig.Language, error) {
	language, err := ssaconfig.ValidateLanguage(raw)
	if err != nil || language == "" || ssaapi.LanguageBuilderCreater[language] == nil {
		return "", fmt.Errorf("unsupported SyntaxFlow debug language")
	}
	return language, nil
}

func legionRuleHash(rule string) string {
	digest := sha256.Sum256([]byte(rule))
	return hex.EncodeToString(digest[:])
}

func legionRuleString(params map[string]any, key string, limit int, required bool) (string, error) {
	value, present := params[key]
	if !present && !required {
		return "", nil
	}
	text, ok := value.(string)
	if !ok || !utf8.ValidString(text) || len(text) > limit || strings.ContainsRune(text, '\x00') || (required && strings.TrimSpace(text) == "") {
		return "", fmt.Errorf("%s must be UTF-8 text of at most %d bytes", key, limit)
	}
	return text, nil
}

func rejectLegionRuleExtraParams(params map[string]any, names ...string) error {
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		allowed[name] = true
	}
	for name := range params {
		if !allowed[name] {
			return fmt.Errorf("SyntaxFlow capability does not accept parameter %q", name)
		}
	}
	return nil
}

// Even direct internal calls must pass the same active immutable-contract gate
// as Execute. Generation fences prevent late workers from entering a new Turn.
func (r *legionServerFocusRuntime) beginRuleOperation(capability string, count bool) (context.Context, uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.workspace == nil || !legionSyntaxFlowRuntimeAvailable() || r.activeFocusContext == nil ||
		r.activeFocusReleaseID == "" || r.activeFocusReleaseID != r.authorizedFocusReleaseID ||
		!r.activeExecutionContract.allowsCapability(capability) {
		return nil, 0, fmt.Errorf("SyntaxFlow capability requires the active authorized Focus Turn and immutable execution contract")
	}
	if count {
		if r.ruleCallCount >= legionSyntaxFlowMaxCalls {
			return nil, 0, fmt.Errorf("SyntaxFlow per-Turn call budget exhausted")
		}
		r.ruleCallCount++
	}
	return r.activeFocusContext, r.activeFocusGeneration, nil
}

func (r *legionServerFocusRuntime) ruleGenerationActive(generation uint64) bool {
	return r.activeFocusGeneration == generation && r.activeFocusReleaseID != "" &&
		r.activeFocusReleaseID == r.authorizedFocusReleaseID && r.activeFocusContext != nil
}

func (r *legionServerFocusRuntime) checkSyntaxFlowRule(params map[string]any) (map[string]any, error) {
	r.ruleMu.Lock()
	defer r.ruleMu.Unlock()
	ctx, generation, err := r.beginRuleOperation(serverFocusCapabilityRuleCheck, true)
	if err != nil {
		return nil, err
	}
	if err := rejectLegionRuleExtraParams(params, "rule"); err != nil {
		return nil, err
	}
	rule, err := legionRuleString(params, "rule", legionSyntaxFlowMaxRuleBytes, true)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, compileErr := compileLegionSyntaxFlowRule(rule)
	status := "valid"
	diagnostics := []string{}
	if compileErr != nil {
		status = "invalid"
		diagnostics = boundedLegionRuleDiagnostics([]string{compileErr.Error()})
	}
	r.mu.Lock()
	active := r.ruleGenerationActive(generation)
	r.mu.Unlock()
	if !active || ctx.Err() != nil {
		return nil, fmt.Errorf("SyntaxFlow check Turn is no longer active")
	}
	return map[string]any{
		"schema_version": legionSyntaxFlowCheckSchema, "rule_sha256": legionRuleHash(rule),
		"status": status, "diagnostics": diagnostics,
	}, nil
}

func compileLegionSyntaxFlowRule(rule string) (*sfvm.SFFrame, error) {
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(rule)
	if err != nil {
		return nil, err
	}
	if frame == nil {
		return nil, fmt.Errorf("SyntaxFlow compiler returned no rule")
	}
	// Runtime policy is also enforced on nested and dynamically compiled
	// frames by WithNativeCallGuard. This pass gives immediate diagnostics.
	for _, code := range frame.Codes {
		if code != nil && code.OpCode == sfvm.OpNativeCall {
			if err := legionSyntaxFlowNativeGuard(code.UnaryStr); err != nil {
				return nil, err
			}
		}
	}
	return frame, nil
}

func legionSyntaxFlowNativeGuard(name string) error {
	// This is intentionally an explicit allowlist. New engine extensions do
	// not acquire workspace, DB, network, or command authority automatically.
	switch name {
	case "const", "self", "len", "root", "var", "slice", "delete", "forbid",
		"getReturns", "getFormalParams", "getFunc", "getCall", "getCallee",
		"searchFunc", "getObject", "getMembers", "getMemberByKey", "getSiblings",
		"typeName", "fullTypeName", "name", "string", "regexp", "strlower", "strupper",
		"opcodes", "sourceCode", "scanPrevious", "scanNext", "scanInstruction", "dataflow",
		"getCfg", "cfgGuards", "cfgDominates", "cfgPostDominates", "cfgReachable",
		"cfgReachPath", "cfgCondition", "cfgConditionValues",
		"getUsers", "getPredecessors", "getActualParams", "getActualParamLen",
		"getCurrentBlueprint", "extendsBy", "getBluePrint", "getParentsBlueprint",
		"getInterfaceBlueprint", "getRootParentBlueprint", "matchRegexpPath",
		"getFullFileName", "FilenameByContent", "versionIn", "uaf", "npd",
		"doubleFree", "heapAlloc", "freeCall", "derefSite", "memLeak", "nullCheck",
		"pointsTo", "aliases":
		return nil
	default:
		return fmt.Errorf("native call %q is unsupported in task-private SyntaxFlow debug", name)
	}
}

func (r *legionServerFocusRuntime) debugSyntaxFlowRule(params map[string]any) (map[string]any, error) {
	r.ruleMu.Lock()
	defer r.ruleMu.Unlock()
	turnCtx, generation, err := r.beginRuleOperation(serverFocusCapabilityRuleDebug, true)
	if err != nil {
		return nil, err
	}
	if err := rejectLegionRuleExtraParams(params, "rule", "language", "source_kind", "paths", "files", "label", "expected"); err != nil {
		return nil, err
	}
	rule, err := legionRuleString(params, "rule", legionSyntaxFlowMaxRuleBytes, true)
	if err != nil {
		return nil, err
	}
	languageText, err := legionRuleString(params, "language", 32, true)
	if err != nil {
		return nil, err
	}
	language, err := normalizeLegionSyntaxFlowLanguage(languageText)
	if err != nil {
		return nil, err
	}
	label, err := legionRuleString(params, "label", 256, false)
	if err != nil {
		return nil, err
	}
	expected, err := legionRuleString(params, "expected", 16, false)
	if err != nil {
		return nil, err
	}
	if expected == "" {
		expected = "observe"
	}
	if expected != "match" && expected != "no_match" && expected != "observe" {
		return nil, fmt.Errorf("expected must be match, no_match, or observe")
	}
	files, sourceKind, sampleOrigin, err := r.ruleDebugSources(params)
	if err != nil {
		return nil, err
	}
	record := legionSyntaxFlowDebugResult{
		SchemaVersion: legionSyntaxFlowDebugSchema, DebugID: uuid.NewString(),
		RuleSHA256: legionRuleHash(rule), SourceKind: sourceKind, SampleOrigin: sampleOrigin,
		SourceSHA256: legionInlineSourceDigest(files), Language: language.String(),
		Label: label, Expected: expected, Status: "completed",
		Matches: []legionSyntaxFlowMatch{}, Diagnostics: []string{},
	}
	if sourceKind == "workspace_files" {
		record.WorkspaceRevision = r.workspace.lockedRevision
	}
	timeout := r.ruleDebugTimeout
	if timeout <= 0 {
		timeout = legionSyntaxFlowDebugTimeout
	}
	workLimit := r.ruleWorkLimit
	if workLimit <= 0 {
		workLimit = legionSyntaxFlowWorkBudget
	}
	ctx, cancel := context.WithTimeout(turnCtx, timeout)
	defer cancel()
	budget := sfvm.NewRuleWorkBudget(workLimit, cancel)
	done := make(chan legionSyntaxFlowDebugResult, 1)
	select {
	case legionSyntaxFlowWorkers <- struct{}{}:
		go func() {
			defer func() { <-legionSyntaxFlowWorkers }()
			done <- executeLegionSyntaxFlowDebug(ctx, rule, language, files, record, budget)
		}()
	case <-ctx.Done():
	}
	select {
	case record = <-done:
	case <-ctx.Done():
		record.Status = legionRuleStoppedStatus(ctx, budget)
		record.Diagnostics = []string{"调试已停止；不把部分结果视为完成。"}
		record.Truncated = true
	}
	// A budget cancel can win the select even if the worker finished first.
	if ctx.Err() != nil {
		record.Status = legionRuleStoppedStatus(ctx, budget)
		record.Truncated = true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ruleGenerationActive(generation) {
		return nil, fmt.Errorf("SyntaxFlow debug Turn is no longer active")
	}
	r.ruleDebugHistory = append(r.ruleDebugHistory, record)
	return legionRuleResultMap(record), nil
}

func legionRuleStoppedStatus(ctx context.Context, budget *sfvm.RuleWorkBudget) string {
	if budget.Exceeded() {
		return "work_budget_exceeded"
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timeout"
	}
	return "cancelled"
}

func executeLegionSyntaxFlowDebug(
	ctx context.Context, rule string, language ssaconfig.Language, files map[string]string,
	record legionSyntaxFlowDebugResult, budget *sfvm.RuleWorkBudget,
) (result legionSyntaxFlowDebugResult) {
	result = record
	phase := "compile_error"
	defer func() {
		if recover() != nil {
			result.Status = phase
			result.Diagnostics = []string{"引擎未能完成本次有界调试。"}
			result.Truncated = true
		}
		if ctx.Err() != nil {
			result.Status = legionRuleStoppedStatus(ctx, budget)
			result.Truncated = true
		}
	}()
	if ctx.Err() != nil {
		return
	}
	frame, err := compileLegionSyntaxFlowRule(rule)
	if err != nil {
		result.Status = "invalid_rule"
		result.Diagnostics = boundedLegionRuleDiagnostics([]string{err.Error()})
		return
	}
	virtual := filesys.NewVirtualFs()
	for _, path := range legionSortedSourcePaths(files) {
		virtual.AddFile(path, files[path])
	}
	programs, err := ssaapi.ParseProjectWithFS(virtual,
		ssaapi.WithLanguage(language), ssaapi.WithContext(ctx), ssaapi.WithMemory(true),
		ssaapi.WithEnableCache(false), ssaapi.WithDisableProgramCache(true),
		ssaapi.WithConcurrency(1), ssaapi.WithStrictMode(true),
	)
	defer func() {
		for _, program := range programs {
			if program != nil && program.Program != nil && program.Program.Cache != nil {
				program.Program.Cache.CloseWithoutSave()
			}
		}
	}()
	if err != nil || len(programs) == 0 {
		result.Status = "compile_error"
		if err != nil {
			result.Diagnostics = boundedLegionRuleDiagnostics([]string{err.Error()})
		} else {
			result.Diagnostics = []string{"没有可编译的源码。"}
		}
		return
	}
	if ctx.Err() != nil {
		return
	}
	phase = "query_error"
	var rejectedNative atomic.Pointer[string]
	guard := func(name string) error {
		err := legionSyntaxFlowNativeGuard(name)
		if err != nil {
			message := err.Error()
			rejectedNative.CompareAndSwap(nil, &message)
		}
		return err
	}
	query, err := ssaapi.QuerySyntaxflow(
		ssaapi.QueryWithPrograms(programs), ssaapi.QueryWithFrame(frame),
		ssaapi.QueryWithContext(ctx), ssaapi.QueryWithWorkBudget(budget),
		ssaapi.QueryWithSFOption(sfvm.WithNativeCallGuard(guard)),
		ssaapi.QueryWithUseCache(false),
		// The default save kind is "none". QueryWithMemory would publish to
		// the global result cache and create risks, so it must not be used.
	)
	// Some recursive predicates intentionally turn a child query error into
	// "no match". A denied capability must still fail the whole debug record.
	if denial := rejectedNative.Load(); denial != nil {
		result.Status = "query_error"
		result.Diagnostics = []string{*denial}
		return
	}
	if err != nil || query == nil {
		result.Status = "query_error"
		if err != nil {
			result.Diagnostics = boundedLegionRuleDiagnostics([]string{err.Error()})
		}
		return
	}
	if diagnostics := query.GetErrors(); len(diagnostics) > 0 {
		result.Status = "query_error"
		result.Diagnostics = boundedLegionRuleDiagnostics(diagnostics)
		return
	}
	variables := append([]string(nil), query.GetAlertVariables()...)
	sort.Strings(variables)
	for _, variable := range variables {
		seen := make(map[int64]bool)
		for _, value := range query.GetValues(variable) {
			if value == nil || seen[value.GetId()] {
				continue
			}
			seen[value.GetId()] = true
			result.MatchCount++
			if len(result.Matches) >= legionSyntaxFlowMaxMatches {
				result.Truncated = true
				continue
			}
			match := legionSyntaxFlowMatch{Variable: trimLegionRuleText(variable, 128)}
			if sourceRange := value.GetRange(); sourceRange != nil && sourceRange.GetEditor() != nil {
				// Anonymous in-memory editors use /relative/path URLs.
				// Only exact members of the authorized input map are emitted.
				path := strings.TrimPrefix(strings.TrimPrefix(filepath.ToSlash(sourceRange.GetEditor().GetUrl()), "./"), "/")
				if _, exists := files[path]; exists {
					match.Path = path
					match.StartLine = sourceRange.GetStart().GetLine()
					match.EndLine = sourceRange.GetEnd().GetLine()
					match.Code = trimLegionRuleText(sourceRange.GetText(), legionSyntaxFlowMaxSnippetBytes)
					result.Truncated = result.Truncated || len(sourceRange.GetText()) > legionSyntaxFlowMaxSnippetBytes
				}
			}
			result.Matches = append(result.Matches, match)
		}
	}
	result.Diagnostics = boundedLegionRuleDiagnostics(query.GetCheckMsg())
	return
}

func (r *legionServerFocusRuntime) ruleDebugSources(params map[string]any) (map[string]string, string, string, error) {
	sourceKind, err := legionRuleString(params, "source_kind", 16, true)
	if err != nil {
		return nil, "", "", err
	}
	switch sourceKind {
	case "inline":
		if _, exists := params["paths"]; exists {
			return nil, "", "", fmt.Errorf("inline debug cannot also select workspace paths")
		}
		files, err := legionRuleStringMap(params["files"])
		if err != nil {
			return nil, "", "", err
		}
		if err := validateLegionInlineFiles(files, false); err != nil {
			return nil, "", "", err
		}
		origin := "generated_or_modified"
		// Exact byte/path comparison against the immutable server-bound map;
		// an unchanged subset is still supplied by the user.
		if len(r.workspace.inlineFiles) > 0 {
			matches := true
			for path, content := range files {
				bound, exists := r.workspace.inlineFiles[path]
				matches = matches && exists && bound == content
			}
			if matches {
				origin = "user_supplied"
			}
		}
		return files, "inline_sample", origin, nil
	case "workspace":
		if _, exists := params["files"]; exists {
			return nil, "", "", fmt.Errorf("workspace debug cannot also supply inline files")
		}
		paths, err := legionRuleStringList(params["paths"], legionSyntaxFlowMaxPaths)
		if err != nil || len(paths) == 0 {
			return nil, "", "", fmt.Errorf("workspace debug requires 1..%d explicit paths", legionSyntaxFlowMaxPaths)
		}
		files := make(map[string]string, len(paths))
		total := 0
		for _, raw := range paths {
			path, err := cleanLegionRuleSourcePath(raw)
			if err != nil {
				return nil, "", "", err
			}
			if _, exists := files[path]; exists {
				return nil, "", "", fmt.Errorf("workspace debug paths must be distinct")
			}
			// resolve confines the path to the bound workspace. Lstat each
			// component additionally rejects even in-workspace symlinks.
			current := r.workspace.root
			for _, part := range strings.Split(path, "/") {
				current = filepath.Join(current, part)
				info, err := os.Lstat(current)
				if err != nil || info.Mode()&os.ModeSymlink != 0 {
					return nil, "", "", fmt.Errorf("workspace debug path is missing or traverses a symlink")
				}
			}
			resolved, _, err := r.workspace.resolve(path)
			if err != nil {
				return nil, "", "", err
			}
			content, err := readLegionRuleSourceFile(resolved, r.workspace.spec.MaxReadBytes)
			if err != nil {
				return nil, "", "", err
			}
			if r.workspace.spec.Kind == legionCodeWorkspaceKindInlineSources {
				if bound, ok := r.workspace.inlineFiles[path]; !ok || bound != content {
					return nil, "", "", fmt.Errorf("inline workspace content no longer matches the bound source")
				}
			}
			files[path] = content
			total += len(content)
			if total > legionInlineSourceMaxTotalBytes {
				return nil, "", "", fmt.Errorf("workspace debug selection exceeds %d bytes", legionInlineSourceMaxTotalBytes)
			}
		}
		return files, "workspace_files", "", nil
	default:
		return nil, "", "", fmt.Errorf("source_kind must be workspace or inline")
	}
}

func readLegionRuleSourceFile(path string, workspaceLimit int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot open selected workspace source")
	}
	defer file.Close()
	info, err := file.Stat()
	limit := int64(legionInlineSourceMaxFileBytes)
	if workspaceLimit > 0 && workspaceLimit < limit {
		limit = workspaceLimit
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return "", fmt.Errorf("workspace debug requires regular files of at most %d bytes", limit)
	}
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(content)) > limit || !utf8.Valid(content) || strings.ContainsRune(string(content), '\x00') {
		return "", fmt.Errorf("workspace debug requires bounded UTF-8 source text")
	}
	return string(content), nil
}

func legionRuleStringMap(value any) (map[string]string, error) {
	if typed, ok := value.(*orderedmap.OrderedMap); ok && typed != nil {
		if len(typed.Keys()) > legionInlineSourceMaxFiles {
			return nil, fmt.Errorf("too many inline source files")
		}
		raw := make(map[string]any, len(typed.Keys()))
		typed.ForEach(func(key string, value any) { raw[key] = value })
		value = raw
	}
	if typed, ok := value.(map[string]string); ok {
		return cloneLegionInlineFiles(typed), nil
	}
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("files must be an explicit map of relative paths to source text")
	}
	result := make(map[string]string, len(typed))
	for key, value := range typed {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("file content must be a string")
		}
		result[key] = text
	}
	return result, nil
}

func legionRuleStringList(value any, limit int) ([]string, error) {
	if typed, ok := value.([]string); ok {
		if len(typed) > limit {
			return nil, fmt.Errorf("too many list items")
		}
		return append([]string(nil), typed...), nil
	}
	typed, ok := value.([]any)
	if !ok || len(typed) > limit {
		return nil, fmt.Errorf("expected a bounded string list")
	}
	result := make([]string, 0, len(typed))
	for _, value := range typed {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("list items must be strings")
		}
		result = append(result, text)
	}
	return result, nil
}

func trimLegionRuleText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	text = text[:limit]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text
}

func boundedLegionRuleDiagnostics(input []string) []string {
	output := make([]string, 0, min(len(input), legionSyntaxFlowMaxDiagnostics))
	for _, diagnostic := range input {
		if len(output) == legionSyntaxFlowMaxDiagnostics {
			break
		}
		output = append(output, trimLegionRuleText(strings.ToValidUTF8(diagnostic, "�"), legionSyntaxFlowMaxDiagnosticBytes))
	}
	return output
}

func legionRuleResultMap(result legionSyntaxFlowDebugResult) map[string]any {
	// A JSON copy prevents model/tool callers from mutating retained evidence.
	raw, _ := json.Marshal(result)
	var mapped map[string]any
	_ = json.Unmarshal(raw, &mapped)
	return mapped
}
