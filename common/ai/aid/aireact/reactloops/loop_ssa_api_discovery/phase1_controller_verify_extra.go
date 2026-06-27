package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

// ControllerVerifyExtraManifest documents persistent extra blocks for acceptance review.
// See prompts/phase1_controller_verify_extra_checklist.md for the human-readable table.
var ControllerVerifyExtraManifest = []struct {
	BlockID   string
	Source    string
	Truncated bool
}{
	{BlockID: "playbook", Source: "phase1_feature_api_unit_playbook.txt", Truncated: false},
	{BlockID: "controller_task", Source: "buildControllerVerifyTaskBlock", Truncated: false},
	{BlockID: "auth_surface_map", Source: "embeddedArtifactsForAgent", Truncated: true},
	{BlockID: "auth_calibration", Source: "embeddedArtifactsForAgent", Truncated: true},
	{BlockID: "failure_semantics", Source: "embeddedArtifactsForAgent", Truncated: true},
	{BlockID: "routing_profile", Source: "embeddedArtifactsForAgent", Truncated: true},
	{BlockID: "probe_context", Source: "buildPhase1VerifyEmbeddedContext", Truncated: true},
	{BlockID: "user_credential_groups", Source: "FormatUserCredentialGroupsInstruction", Truncated: false},
	{BlockID: "fs_tool_params", Source: "ssaDiscoveryFSBuiltinToolParamsHint", Truncated: false},
	{BlockID: "http_tool_params", Source: "ssaDiscoveryHTTPBuiltinToolParamsHint", Truncated: false},
	{BlockID: "frontend_api_hints", Source: "buildFrontendAPIHintsBlock", Truncated: true},
}

// buildControllerVerifyExtra assembles the full persistent extra per ControllerVerifyExtraManifest.
func buildControllerVerifyExtra(rt *Runtime, job ControllerVerifyJob) string {
	var parts []string
	parts = append(parts, buildControllerVerifyTaskBlock(job))
	parts = append(parts, embeddedArtifactsForAgent(rt,
		store.AuthSurfaceMapPath(rt.WorkDir),
		store.AuthCalibrationPath(rt.WorkDir),
		store.FailureSemanticsPath(rt.WorkDir),
		store.RoutingProfilePath(rt.WorkDir),
	))
	parts = append(parts, buildPhase1VerifyEmbeddedContext(rt))
	parts = append(parts, FormatUserCredentialGroupsInstruction(rt))
	if scope := formatPartialAuthProbeScope(rt, job); scope != "" {
		parts = append(parts, scope)
	}
	if block := buildPrefixCandidatesBlock(rt, job); block != "" {
		parts = append(parts, block)
	}
	if block := buildServletRoutingMapBlock(rt); block != "" {
		parts = append(parts, block)
	}
	if block := buildResolvedRoutesBlock(rt, job); block != "" {
		parts = append(parts, block)
	}
	realm := InferAuthRealmForFeatureJob(rt, job)
	if realm != "" {
		parts = append(parts, "## required_auth_realm\n"+realm+"\nUse discovery_select_auth_credential matching this realm.")
	}

	if block := buildFrontendAPIHintsBlock(rt, job); block != "" {
		parts = append(parts, block)
	}
	return strings.Join(parts, "\n\n")
}

func buildControllerVerifyTaskBlock(job ControllerVerifyJob) string {
	payload := map[string]any{
		"entry_file":       job.EntryFile,
		"controller_file":  job.EntryFile,
		"feature_id":       job.FeatureID,
		"feature_label":    job.FeatureLabel,
		"package_patterns": job.PackagePatterns,
		"static_hints":     job.StaticHints,
		"hint_count":       len(job.StaticHints),
		"code_root_note":   "read_file 可使用绝对路径或相对 code_root 的路径；需要时可读其他配置/父类文件",
	}
	b, _ := json.MarshalIndent(payload, "", "  ")
	return "## controller_task\n```json\n" + string(b) + "\n```"
}

func extraManifestBlockIDs() []string {
	out := make([]string, len(ControllerVerifyExtraManifest))
	for i, row := range ControllerVerifyExtraManifest {
		out[i] = row.BlockID
	}
	return out
}

func logControllerVerifyExtraManifest(job ControllerVerifyJob) string {
	return fmt.Sprintf("controller_verify_extra blocks=%v entry_file=%s feature=%s hints=%d",
		extraManifestBlockIDs(), job.EntryFile, job.FeatureID, len(job.StaticHints))
}
