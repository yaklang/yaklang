package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

// Waiver kinds recorded in DiscoverySession.PipelineWaiverJSON.
const (
	waiverTargetUnreachable      = "target_unreachable"
	waiverAuthCredentialsMissing = "auth_credentials_missing"
	waiverDeepMiningNoTargets    = "deep_mining_no_targets"
	waiverGreyboxSkipped         = "greybox_skipped"
	waiverStaticVerifySkipped    = "static_verify_skipped"
)

const (
	minDiscoveryReportBytes = 128
	minStepReportBytes      = 64
	minFinalReportBytes     = 64
	highChecklistPriority   = 3
)

func hasPipelineWaiver(sess *store.DiscoverySession, kind string) bool {
	if sess == nil {
		return false
	}
	return strings.Contains(sess.PipelineWaiversJSON, fmt.Sprintf(`"%s"`, kind))
}

func recordPipelineWaiver(rt *Runtime, phaseIndex int, kind, reason string) error {
	if rt == nil || rt.Session == nil || rt.Repo == nil {
		return nil
	}
	entry := fmt.Sprintf(`{"phase":%d,"kind":%q,"reason":%q,"ts":%d}`, phaseIndex, kind, reason, time.Now().Unix())
	existing := strings.TrimSpace(rt.Session.PipelineWaiversJSON)
	if existing == "" {
		rt.Session.PipelineWaiversJSON = "[" + entry + "]"
	} else {
		rt.Session.PipelineWaiversJSON = strings.TrimSuffix(existing, "]") + "," + entry + "]"
	}
	return rt.Repo.UpdateSession(rt.Session)
}

func authLikelyRequired(rt *Runtime) bool {
	if rt == nil || rt.Session == nil {
		return false
	}
	if !rt.Session.TargetReachable {
		return false
	}
	creds, err := rt.Repo.ListVerifiedAuthCredentials(rt.Session.ID)
	return err == nil && len(creds) == 0
}

// EnforcePhaseContract verifies legacy pipeline contract artifacts per phase.
func EnforcePhaseContract(rt *Runtime, pl *PipelineState, phaseIndex int) error {
	if rt == nil {
		return nil
	}
	switch phaseIndex {
	case 1:
		return enforcePhase1Contract(rt)
	case 2:
		return enforcePhase2Contract(rt, pl)
	case 3:
		return enforcePhase3Contract(rt, pl)
	case 4:
		return enforcePhase4Contract(rt, pl)
	case 5:
		return enforcePhase5Contract(rt, pl)
	default:
		return nil
	}
}

// failPipelineOnContract aborts the pipeline when a phase contract is broken.
func failPipelineOnContract(r aicommon.AIInvokeRuntime, rt *Runtime, phaseIndex int, err error, op *reactloops.InitTaskOperator) {
	if r != nil && err != nil {
		r.AddToTimeline("[ssa_pipeline]", fmt.Sprintf("phase%d_contract_failed: %v", phaseIndex, err))
	}
	if op != nil {
		op.Failed(err)
	}
}

func enforcePhase1Contract(rt *Runtime) error {
	if rt == nil || rt.WorkDir == "" {
		return utils.Error("nil runtime or workdir")
	}
	if _, err := os.Stat(store.Phase1PrepBundlePath(rt.WorkDir)); err != nil {
		return utils.Errorf("phase1_prep_bundle 缺失：%v", err)
	}
	if _, err := os.Stat(store.RouteCandidatesPath(rt.WorkDir)); err != nil {
		return utils.Errorf("route_candidates 缺失：%v", err)
	}
	// NOTE: code_reading_plan.json 是旧架构产物（Phase1B 分阶段代码阅读计划）。
	// 新架构（GranularFeaturePipeline）已由 unified_endpoints -> route_candidates +
	// feature_inventory -> coverage_signal_decision + feature_api_map 提供覆盖率决策，
	// 不再强依赖该文件。如后续步骤（如 probe_backfill、auth_multi）需要读取，
	// 应改用 unified_endpoints 或 feature_api_map 作为数据源。
	if rt.Session != nil && !rt.Session.TargetReachable {
		_ = recordPipelineWaiver(rt, 1, waiverTargetUnreachable, "target unreachable at phase1 contract")
	}
	return nil
}

func enforcePhase2Contract(rt *Runtime, pl *PipelineState) error {
	if rt == nil {
		return nil
	}
	if pl == nil {
		return utils.Error("nil pipeline state")
	}
	return nil
}

func enforcePhase3Contract(rt *Runtime, pl *PipelineState) error {
	if rt == nil {
		return nil
	}
	if pl == nil {
		return utils.Error("nil pipeline state")
	}
	return nil
}

func enforcePhase4Contract(rt *Runtime, pl *PipelineState) error {
	if rt == nil {
		return nil
	}
	if pl == nil {
		return utils.Error("nil pipeline state")
	}
	return enforcePhase4DynamicContract(rt, pl)
}

func enforcePhase5Contract(rt *Runtime, pl *PipelineState) error {
	if rt == nil {
		return nil
	}
	if pl == nil {
		return utils.Error("nil pipeline state")
	}
	return nil
}

func enforcePhase4DynamicContract(rt *Runtime, pl *PipelineState) error {
	if rt == nil || rt.WorkDir == "" || pl == nil {
		return utils.Error("nil runtime/pipeline state or empty workdir")
	}
	for _, p := range []string{
		pl.GetStep0ReportPath(),
		pl.GetStep1AuthReportPath(),
		pl.GetStep2VerifyReportPath(),
		pl.GetStep3GreyboxReportPath(),
	} {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if _, statErr := os.Stat(p); statErr != nil {
			return utils.Errorf("phase4 step report missing: %s", p)
		}
	}
	if !hasVerifiedApiWithProbeURL(rt) {
		_ = recordPipelineWaiver(rt, 4, waiverDeepMiningNoTargets, "no probe targets")
		return nil
	}
	if !pl.GetGreyboxExecuted() {
		if rt.Session != nil && rt.Session.TargetReachable {
			return utils.Error("phase4 requires greybox execution when verified API probe targets exist")
		}
		_ = recordPipelineWaiver(rt, 4, waiverDeepMiningNoTargets, "no greybox execution")
		return nil
	}
	if rt.Session != nil && rt.Session.TargetReachable && authLikelyRequired(rt) {
		_ = recordPipelineWaiver(rt, 4, waiverAuthCredentialsMissing, "Phase1 未写入 auth_credentials；Phase4 跳过重复鉴权")
	}
	if rt.Phase4Mode() == Phase4ModeDeepMining && pl.CountDeepMiningDone() < countVerifiedApisWithProbeURL(rt) {
		return utils.Errorf("deep_mining 未完成：verified probe targets=%d deep_mining_done=%d", countVerifiedApisWithProbeURL(rt), pl.CountDeepMiningDone())
	}
	return nil
}

func countVerifiedApisWithProbeURL(rt *Runtime) int {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return 0
	}
	rows, _ := rt.Repo.ListVerifiedHttpApis(rt.Session.ID)
	n := 0
	for _, r := range rows {
		if r.Verified && strings.TrimSpace(r.FullSampleURL) != "" {
			n++
		}
	}
	return n
}

func hasVerifiedApiWithProbeURL(rt *Runtime) bool {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return false
	}
	rows, _ := rt.Repo.ListVerifiedHttpApis(rt.Session.ID)
	for _, r := range rows {
		if r.Verified && strings.TrimSpace(r.FullSampleURL) != "" {
			return true
		}
	}
	return false
}

// writePhase1ContractStubArtifacts writes minimum artifacts for contract tests.
func writePhase1ContractStubArtifacts(workDir string) error {
	dir := filepath.Join(workDir, store.SubDirName())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	stubs := map[string]string{
		store.ProjectProfilePath(workDir):        `{"schema_version":1,"files":[{"rel_path":"x.java","category":"code"}],"frameworks":[{"id":"spring","label":"Spring"}],"context_path":"unknown"}`,
		store.ApiCatalogPath(workDir):            `{"schema_version":1,"entries":[{"method":"GET","path_pattern":"/api/x","full_url":"http://127.0.0.1/api/x","code_evidence":"test"}]}`,
		store.Phase1DiscoveryReportPath(workDir): "# Phase1 stub report\n\ncontract test stub with enough content to satisfy min bytes check for pipeline contract.\n",
		store.TechArchitecturePath(workDir):      `{"schema_version":1,"language":"java","system_summary":"stub tech arch for contract test with enough detail."}`,
		store.BusinessFunctionMapPath(workDir):   `{"schema_version":1,"functions":{"stub":{"description":"stub","scope_paths":["src/main/java"]}},"coverage":{"policy":"java_package_units_must_be_covered","total_required":1,"covered":1,"complete":true}}`,
		store.JavaBusinessScopeInventoryPath(workDir): `{"schema_version":1,"language":"java","layout":"single_module","modules":[{"module_root":".","scope_units":[{"id":"x","kind":"java_package","path":"src/main/java"}]}],"coverage_policy":{"required_kinds":["java_package"]},"stats":{"java_package_units":1}}`,
		store.FeatureInventoryPath(workDir):           `{"schema_version":1,"features":[{"feature_id":"stub","label":"stub","surface_kind":"http_api","package_patterns":["src/main/java"],"entry_files":["mod/StubController.java"]}],"coverage":{"policy":"code_unit_registry_entry_files","total_required":1,"covered":1,"complete":true}}`,
		store.FeatureApiMapPath(workDir):              `{"schema_version":1,"features":[{"feature_id":"stub","api_count":1,"processed":true,"apis":[{"method":"GET","path_pattern":"/a","verified":true,"full_sample_url":"http://127.0.0.1:8080/a","verdict_reason":"hit"}]}]}`,
		store.CodeUnitRegistryPath(workDir):           `{"schema_version":1,"units":[{"rel_path":"mod/StubController.java","kind_hint":"http_entry"}]}`,
		store.FeatureWorkProgressPath(workDir):        `{"entries":[{"entry_file":"mod/StubController.java","job_kind":"http_api","status":"completed"}]}`,
		store.FailureSemanticsPath(workDir):           `{"schema_version":1,"categories":[{"kind":"wrong_path","body_patterns":["interfaceNotFound"],"route_verdict":"wrong_route"},{"kind":"unauthorized","status_codes":[401]},{"kind":"success","body_patterns":["statusCode"]}]}`,
		store.RoutingProfilePath(workDir):             `{"schema_version":1,"validation_status":"confirmed","url_spaces":[{"id":"default","mount_prefix":"/"}],"effective_bases":[{"space_id":"default","base_url":"http://127.0.0.1:8080/"}]}`,
	}
	for path, body := range stubs {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func fileExistsMinBytes(path string, minBytes int) error {
	if minBytes <= 0 {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() < int64(minBytes) {
		return fmt.Errorf("file %s is %d bytes, below minimum %d", path, info.Size(), minBytes)
	}
	return nil
}

func loadRouteCandidates(workDir string) ([]RouteCandidate, error) {
	if workDir == "" {
		return nil, fmt.Errorf("empty workdir")
	}
	path := store.RouteCandidatesPath(workDir)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []RouteCandidate
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []RouteCandidate{}, nil
	}
	return out, nil
}
