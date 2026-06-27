package loop_ssa_api_discovery

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/phase1_controller_verify_playbook.txt
var phase1ControllerVerifyPlaybook string

//go:embed prompts/phase1_feature_api_unit_playbook.txt
var phase1FeatureApiUnitPlaybook string

//go:embed prompts/phase1_cms_servlet_routing_playbook.txt
var phase1CmsServletRoutingPlaybook string

const (
	defaultControllerVerifyConcurrent = 4
	controllerVerifyCommittedLoopKey  = "controller_verification_committed"
)

// ControllerVerifyJob is kept as alias for HTTP API unit jobs in tests.
type ControllerVerifyJob = FeatureWorkJob

// HttpApiUnitResult is the committed result for one http_api entry file.
type HttpApiUnitResult struct {
	EntryFile      string            `json:"entry_file"`
	ControllerFile string            `json:"controller_file,omitempty"`
	FeatureID      string            `json:"feature_id"`
	FeatureLabel   string            `json:"feature_label,omitempty"`
	NoApiReason    string            `json:"no_api_reason,omitempty"`
	Apis           []FeatureApiEntry `json:"apis,omitempty"`
}

// ControllerVerifyEntry is legacy alias for HttpApiUnitResult.
type ControllerVerifyEntry = HttpApiUnitResult

func runPhase1HttpApiUnitReAct(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, job FeatureWorkJob) (HttpApiUnitResult, error) {
	extra := buildControllerVerifyExtra(rt, job)
	loop, err := buildPhase1ControllerVerifyLoop(r, rt, job, extra)
	if err != nil {
		return HttpApiUnitResult{}, err
	}
	scope := ControllerVerifyScope{
		ControllerFile:  job.EntryFile,
		FeatureID:       job.FeatureID,
		FeatureLabel:    job.FeatureLabel,
		PackagePatterns: job.PackagePatterns,
		RouteKeys:       controllerVerifyRouteKeys(job.StaticHints),
	}
	setLoopControllerScope(loop, scope)
	subName := "phase1_http_api_unit_" + controllerVerifySubSlug(job.EntryFile)
	if err := runPhase1ReActLoop(task, subName, loop); err != nil {
		return HttpApiUnitResult{}, err
	}
	raw := strings.TrimSpace(loop.Get(controllerVerifyCommittedLoopKey))
	if raw == "" {
		return HttpApiUnitResult{}, utils.Error("http api unit not committed")
	}
	var entry HttpApiUnitResult
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		return HttpApiUnitResult{}, err
	}
	normalizeHttpApiUnitResult(&entry, job)
	if err := validateHttpApiUnitResult(rt, &entry, job); err != nil {
		return HttpApiUnitResult{}, err
	}
	inv, _ := loadFeatureInventory(rt.WorkDir)
	if err := mergeAndPersistHttpApiUnitResult(rt, inv, entry); err != nil {
		return HttpApiUnitResult{}, err
	}
	return entry, nil
}

func normalizeHttpApiUnitResult(entry *HttpApiUnitResult, job FeatureWorkJob) {
	if entry == nil {
		return
	}
	if strings.TrimSpace(entry.EntryFile) == "" {
		entry.EntryFile = job.EntryFile
	}
	if strings.TrimSpace(entry.ControllerFile) == "" {
		entry.ControllerFile = entry.EntryFile
	}
	if strings.TrimSpace(entry.FeatureID) == "" {
		entry.FeatureID = job.FeatureID
	}
	if strings.TrimSpace(entry.FeatureLabel) == "" {
		entry.FeatureLabel = job.FeatureLabel
	}
}

func buildPhase1ControllerVerifyLoop(r aicommon.AIInvokeRuntime, rt *Runtime, job FeatureWorkJob, extra string) (*reactloops.ReActLoop, error) {
	preset := phase1ControllerAgentBaseOptions(r, rt, job, extra, phase1FeatureApiUnitPlaybook)
	preset = append(preset, phase1AgentSearchOptions()...)
	preset = append(preset,
		buildUpsertHttpEndpoint(),
		buildListAuthCredentialsAction(),
		buildSelectAuthCredentialAction(),
		buildAuthAwareHTTPAction(r, rt, nil),
		buildDiscoveryFetchCsrfToken(r, rt),
		buildRecordProbeAttempt(),
		buildDiscoveryProbeApiCandidate(r),
		buildFinalizeHttpApiUnit(rt, job),
		buildFinalizeControllerVerification(rt, job),
		buildBlockedDirectlyAnswer("finalize_http_api_unit"),
		buildBlockedDirectlyAnswer("finalize_controller_verification"),
	)
	preset = append(preset, phase1FeatureVerifyHTTPActionOptions()...)
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_FEATURE_VERIFY, r, preset...)
}

func phase1ControllerAgentBaseOptions(r aicommon.AIInvokeRuntime, rt *Runtime, job FeatureWorkJob, extra string, playbook string) []reactloops.ReActLoopOption {
	if strings.TrimSpace(playbook) == "" {
		playbook = phase1FeatureApiUnitPlaybook
	}
	persistent := strings.TrimSpace(playbook)
	if strings.TrimSpace(phase1CmsServletRoutingPlaybook) != "" {
		persistent += "\n\n" + strings.TrimSpace(phase1CmsServletRoutingPlaybook)
	}
	if extra != "" {
		persistent += "\n\n" + strings.TrimSpace(extra)
	}
	persistent += "\n\n" + strings.TrimSpace(ssaDiscoveryFSBuiltinToolParamsHint)
	persistent += "\n\n" + strings.TrimSpace(ssaDiscoveryHTTPBuiltinToolParamsHint)
	return []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(ssaDiscoveryMaxIterations(r)),
		reactloops.WithAllowToolCall(true),
		reactloops.WithAllowRAG(true),
		reactloops.WithAllowAIForge(false),
		reactloops.WithPersistentInstruction(persistent),
		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			base := ""
			if rt != nil && rt.Session != nil {
				base = EffectiveTargetBaseURL(rt.Session)
			}
			return fmt.Sprintf(`<|PHASE1_HTTP_API_%s|>
session: %s
target_base: %s
code_root: %s
target_reachable: %v
entry_file: %s
feature_id: %s
feedback:
%s
<|END_%s|>`,
				nonce,
				loop.Get("discovery_session_uuid"),
				base,
				loopGetCodeRoot(rt),
				loopGetTargetReachable(rt),
				job.EntryFile,
				job.FeatureID,
				feedbacker.String(),
				nonce,
			), nil
		}),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			setRuntime(loop, rt)
			if rt != nil && rt.Session != nil {
				loop.Set("discovery_session_uuid", rt.Session.UUID)
				loop.Set("discovery_sqlite_path", rt.SQLitePath)
				loop.Set("discovery_code_root", rt.Session.CodeRootPath)
			}
			op.NextAction("read_file")
		}),
		buildDiscoveryGetStatus(),
		buildDiscoveryReadSessionData(),
		buildCodeReadingReadFileAudit(rt),
	}
}

func buildFinalizeHttpApiUnit(rt *Runtime, job FeatureWorkJob) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_http_api_unit",
		"Commit HTTP API unit verification result and exit.",
		[]aitool.ToolOption{aitool.WithStringParam("result_json", aitool.WithParam_Required(true))},
		nil,
		finalizeHttpApiUnitHandler(rt, job),
	)
}

func buildFinalizeControllerVerification(rt *Runtime, job FeatureWorkJob) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_controller_verification",
		"Commit controller verification result and exit (legacy alias).",
		[]aitool.ToolOption{aitool.WithStringParam("result_json", aitool.WithParam_Required(true))},
		nil,
		finalizeHttpApiUnitHandler(rt, job),
	)
}

func finalizeHttpApiUnitHandler(rt *Runtime, job FeatureWorkJob) func(*reactloops.ReActLoop, *aicommon.Action, *reactloops.LoopActionHandlerOperator) {
	return func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		raw := strings.TrimSpace(action.GetString("result_json"))
		var entry HttpApiUnitResult
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			op.Feedback("invalid result_json: " + err.Error())
			op.Continue()
			return
		}
		normalizeHttpApiUnitResult(&entry, job)
		if err := validateHttpApiUnitResult(rt, &entry, job); err != nil {
			op.Feedback("validation: " + err.Error())
			op.Continue()
			return
		}
		b, _ := json.MarshalIndent(entry, "", "  ")
		loop.Set(controllerVerifyCommittedLoopKey, string(b))
		op.Exit()
	}
}

func validateHttpApiUnitResult(rt *Runtime, entry *HttpApiUnitResult, job FeatureWorkJob) error {
	if entry == nil {
		return utils.Error("nil http api unit entry")
	}
	if strings.TrimSpace(entry.EntryFile) == "" {
		return utils.Error("entry_file required")
	}
	if strings.TrimSpace(entry.FeatureID) == "" {
		return utils.Error("feature_id required")
	}
	if len(entry.Apis) == 0 {
		if strings.TrimSpace(entry.NoApiReason) == "" {
			return utils.Error("no_api_reason required when apis is empty")
		}
		return nil
	}
	liveTarget := rt != nil && rt.Session != nil && rt.Session.TargetReachable
	inv, _ := loadFeatureInventory(rt.WorkDir)
	for i, a := range entry.Apis {
		if a.Method == "" || a.PathPattern == "" {
			return utils.Errorf("apis[%d] method and path_pattern required", i)
		}
		if !strings.HasPrefix(strings.TrimSpace(a.PathPattern), "/") {
			return utils.Errorf("apis[%d] path_pattern must be absolute, got %q", i, a.PathPattern)
		}
		if hc := strings.TrimSpace(a.HandlerClass); hc != "" && inv != nil {
			var feat FeatureInventoryEntry
			for _, f := range inv.Features {
				if f.FeatureID == job.FeatureID {
					feat = f
					break
				}
			}
			if feat.FeatureID != "" && !handlerMatchesFeature(hc, job.EntryFile, feat) {
				return utils.Errorf("apis[%d] handler_class %q does not match entry_file %q", i, hc, job.EntryFile)
			}
		}
		if liveTarget && a.Verified && strings.TrimSpace(a.FullSampleURL) == "" {
			return utils.Errorf("apis[%d] verified=true requires full_sample_url after live HTTP probe", i)
		}
		if liveTarget && a.Verified && strings.TrimSpace(a.VerdictReason) == "" {
			return utils.Errorf("apis[%d] verified=true requires verdict_reason from HTTP probe", i)
		}
	}
	return nil
}

func validateControllerVerifyEntry(rt *Runtime, entry *ControllerVerifyEntry, job ControllerVerifyJob) error {
	return validateHttpApiUnitResult(rt, entry, job)
}

func mergeHttpApiUnitResultsIntoFeatureMap(inv *FeatureInventoryV1, apiMap *FeatureApiMapV1, results []HttpApiUnitResult) {
	legacy := make([]ControllerVerifyEntry, len(results))
	copy(legacy, results)
	mergeControllerResultsIntoFeatureMap(inv, apiMap, legacy)
}

func mergeControllerResultsIntoFeatureMap(inv *FeatureInventoryV1, apiMap *FeatureApiMapV1, results []ControllerVerifyEntry) {
	if apiMap == nil || len(results) == 0 {
		return
	}
	for _, r := range results {
		featID := strings.TrimSpace(r.FeatureID)
		if featID == "" {
			continue
		}
		label := strings.TrimSpace(r.FeatureLabel)
		if label == "" && inv != nil {
			for _, f := range inv.Features {
				if f.FeatureID == featID {
					label = f.Label
					break
				}
			}
		}
		entry := FeatureApiMapEntry{
			FeatureID: featID,
			Label:     label,
			Processed: true,
		}
		for i := range apiMap.Features {
			if apiMap.Features[i].FeatureID == featID {
				entry = apiMap.Features[i]
				entry.Processed = true
				if label != "" {
					entry.Label = label
				}
				break
			}
		}
		entry.Apis = append(entry.Apis, r.Apis...)
		entry.Apis = dedupeFeatureApis(entry.Apis)
		entry.ApiCount = len(entry.Apis)
		if strings.TrimSpace(r.NoApiReason) != "" {
			entry.NoApiReason = strings.TrimSpace(r.NoApiReason)
		}
		if entry.ApiCount == 0 {
			if sk := surfaceKindForFeature(inv, featID); sk == SurfaceKindCodeOnly {
				entry.NoApiReason = "code_only feature"
			} else if strings.TrimSpace(entry.NoApiReason) == "" {
				entry.NoApiReason = "no APIs from http api unit verify"
			}
		}
		mergeFeatureApiMapEntry(apiMap, entry)
	}
}

func surfaceKindForFeature(inv *FeatureInventoryV1, featureID string) string {
	if inv == nil {
		return ""
	}
	for _, f := range inv.Features {
		if f.FeatureID == featureID {
			return strings.TrimSpace(f.SurfaceKind)
		}
	}
	return ""
}

func dedupeFeatureApis(apis []FeatureApiEntry) []FeatureApiEntry {
	seen := map[string]struct{}{}
	var out []FeatureApiEntry
	for _, a := range apis {
		k := routeKey(a.Method, a.PathPattern)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, a)
	}
	return out
}

func controllerVerifyRouteKeys(hints []StaticRouteHint) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, h := range hints {
		k := routeKey(h.Method, h.PathPattern)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

var controllerVerifySubSlugSanitizer = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func controllerVerifySubSlug(entryFile string) string {
	base := strings.TrimSuffix(filepath.Base(entryFile), ".java")
	s := controllerVerifySubSlugSanitizer.ReplaceAllString(base, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		s = "unit"
	}
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

func controllerVerifyConcurrent() int {
	n := defaultControllerVerifyConcurrent
	s := strings.TrimSpace(os.Getenv("YAK_SSA_API_DISCOVERY_CONTROLLER_VERIFY_CONCURRENT"))
	if s == "" {
		return n
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return n
	}
	if v > 16 {
		return 16
	}
	return v
}

func collectControllerVerifyJobs(rt *Runtime, inv *FeatureInventoryV1) ([]ControllerVerifyJob, error) {
	jobs, err := collectFeatureWorkJobs(rt, inv)
	if err != nil {
		return nil, err
	}
	var httpJobs []ControllerVerifyJob
	for _, j := range jobs {
		if strings.TrimSpace(j.SurfaceKind) == SurfaceKindHTTPAPI {
			httpJobs = append(httpJobs, j)
		}
	}
	return httpJobs, nil
}

func runPhase1ControllerVerifyChain(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	return runPhase1FeatureWorkChain(r, task, rt)
}
