package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"google.golang.org/protobuf/proto"
)

const (
	legionForgeReleaseSchemaV1  = "legion.ai-forge-release/v1"
	legionForgeExecutorConfigV1 = "config_forge_v1"
	legionForgeAdvisoryProfile  = "advisory.v1"
	legionForgeReportProfile    = "report.v1"
	legionForgeHTTPProfile      = "http_assessment.v1"
	legionForgeDiscoveryProfile = "discovery.v1"
	legionForgeEvidenceProfile  = "evidence.v1"
	maxLegionForgeParameters    = 64
	maxLegionForgeTools         = 32
	maxLegionForgeReleaseBytes  = 512 << 10
)

var (
	legionForgeReportTools    = []string{"parse_office_to_text", "query_file_meta", "read_file", "read_file_lines"}
	legionForgeHTTPTools      = []string{"do_http_request", "send_http_request_by_url", "simple_crawler", "url_content_summary", "web_fingerprint"}
	legionForgeDiscoveryTools = []string{"dns_lookup", "tcp_connect_scan"}
	legionForgeEvidenceTools  = []string{"parse_android_package", "parse_office_to_text", "parse_packet_capture", "query_file_meta", "read_file", "read_file_lines"}
)

func validateContextForgeRelease(release *aiv1.ContextForgeRelease) error {
	if release == nil {
		return nil
	}
	if strings.TrimSpace(release.GetSchemaVersion()) != legionForgeReleaseSchemaV1 {
		return fmt.Errorf("Forge release has unsupported schema %q", release.GetSchemaVersion())
	}
	for label, value := range map[string]string{
		"release_id": release.GetReleaseId(),
		"forge_id":   release.GetForgeId(),
		"version":    release.GetVersion(),
		"name":       release.GetName(),
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("Forge release %s is required", label)
		}
	}
	if strings.TrimSpace(release.GetExecutorKind()) != legionForgeExecutorConfigV1 {
		return fmt.Errorf("Forge release has unsupported executor %q", release.GetExecutorKind())
	}
	profile := strings.TrimSpace(release.GetCapabilityProfile())
	if profile != legionForgeAdvisoryProfile && profile != legionForgeReportProfile && profile != legionForgeHTTPProfile && profile != legionForgeDiscoveryProfile && profile != legionForgeEvidenceProfile {
		return fmt.Errorf("Forge release capability profile %q is not supported by this node", release.GetCapabilityProfile())
	}
	if len(release.GetParameters()) > maxLegionForgeParameters || len(release.GetDeclaredToolNames()) > maxLegionForgeTools {
		return fmt.Errorf("Forge release exceeds bounded input limits")
	}
	switch profile {
	case legionForgeDiscoveryProfile:
		if !equalContextForgeStrings(release.GetDeclaredToolNames(), legionForgeDiscoveryTools) {
			return fmt.Errorf("discovery Forge release must declare exact discovery tools")
		}
		if _, _, err := legionForgeDiscoveryParameters(release); err != nil {
			return err
		}
	case legionForgeEvidenceProfile:
		if !equalContextForgeStrings(release.GetDeclaredToolNames(), legionForgeEvidenceTools) {
			return fmt.Errorf("evidence Forge release must declare exact evidence tools")
		}
	case legionForgeAdvisoryProfile:
		if len(release.GetDeclaredToolNames()) != 0 {
			return fmt.Errorf("advisory Forge release cannot declare executable tools")
		}
	case legionForgeReportProfile:
		if !equalContextForgeStrings(release.GetDeclaredToolNames(), legionForgeReportTools) {
			return fmt.Errorf("report Forge release must declare the exact managed report tools")
		}
	case legionForgeHTTPProfile:
		if !equalContextForgeStrings(release.GetDeclaredToolNames(), legionForgeHTTPTools) {
			return fmt.Errorf("HTTP Forge release must declare the exact bounded HTTP tools")
		}
	}
	if len(release.GetInputSchemaJson()) > 0 && !json.Valid(release.GetInputSchemaJson()) {
		return fmt.Errorf("Forge release input schema is invalid JSON")
	}
	if !normalizedContextForgeParameters(release.GetParameters(), profile) {
		return fmt.Errorf("Forge release parameters must be unique and sorted")
	}
	if !normalizedContextForgeTools(release.GetDeclaredToolNames()) {
		return fmt.Errorf("Forge release declared tools must be unique and sorted")
	}
	if proto.Size(release) > maxLegionForgeReleaseBytes {
		return fmt.Errorf("Forge release exceeds definition size limit")
	}
	definitionWant := strings.TrimSpace(release.GetDefinitionSha256())
	if len(definitionWant) != sha256.Size*2 || strings.ToLower(definitionWant) != definitionWant {
		return fmt.Errorf("Forge release definition_sha256 is invalid")
	}
	if _, err := hex.DecodeString(definitionWant); err != nil {
		return fmt.Errorf("Forge release definition_sha256 is invalid")
	}
	definitionGot, err := contextForgeDefinitionSHA256(release)
	if err != nil {
		return err
	}
	if definitionGot != definitionWant {
		return fmt.Errorf("Forge release %q definition identity mismatch", release.GetReleaseId())
	}
	want := strings.TrimSpace(release.GetSha256())
	if len(want) != sha256.Size*2 || strings.ToLower(want) != want {
		return fmt.Errorf("Forge release sha256 is invalid")
	}
	if _, err := hex.DecodeString(want); err != nil {
		return fmt.Errorf("Forge release sha256 is invalid")
	}
	got, err := contextForgeReleaseSHA256(release)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("Forge release %q identity mismatch", release.GetReleaseId())
	}
	return nil
}

func contextForgeDefinitionSHA256(release *aiv1.ContextForgeRelease) (string, error) {
	if release == nil {
		return "", fmt.Errorf("Forge release is required")
	}
	clone := proto.Clone(release).(*aiv1.ContextForgeRelease)
	clone.Sha256 = ""
	clone.DefinitionSha256 = ""
	clone.Parameters = nil
	raw, err := (proto.MarshalOptions{Deterministic: true}).Marshal(clone)
	if err != nil {
		return "", fmt.Errorf("encode Forge release definition: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func contextForgeReleaseSHA256(release *aiv1.ContextForgeRelease) (string, error) {
	if release == nil {
		return "", fmt.Errorf("Forge release is required")
	}
	clone := proto.Clone(release).(*aiv1.ContextForgeRelease)
	clone.Sha256 = ""
	raw, err := (proto.MarshalOptions{Deterministic: true}).Marshal(clone)
	if err != nil {
		return "", fmt.Errorf("encode Forge release: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func normalizedContextForgeParameters(values []*aiv1.ContextForgeParameter, profile string) bool {
	keys := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == nil {
			return false
		}
		key := strings.TrimSpace(value.GetKey())
		kind := strings.TrimSpace(value.GetValueKind())
		if key == "" || key != value.GetKey() || strings.TrimSpace(value.GetValue()) == "" || (kind != "string" && kind != "text" && kind != "resource") {
			return false
		}
		if kind == "resource" && profile != legionForgeReportProfile && profile != legionForgeEvidenceProfile {
			return false
		}
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	return equalContextForgeStrings(keys, sorted)
}

func normalizedContextForgeTools(values []string) bool {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" || name != value {
			return false
		}
		if _, exists := seen[name]; exists {
			return false
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}
	sorted := append([]string(nil), normalized...)
	sort.Strings(sorted)
	return equalContextForgeStrings(normalized, sorted)
}

func equalContextForgeStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func buildContextForgeBlueprint(
	release *aiv1.ContextForgeRelease,
) (*aiforge.YakForgeBlueprintConfig, *aiforge.ForgeBlueprint, []*ypb.ExecParamItem, error) {
	if err := validateContextForgeRelease(release); err != nil {
		return nil, nil, nil, err
	}
	disableTools := release.GetCapabilityProfile() == legionForgeAdvisoryProfile
	config := aiforge.NewYakForgeBlueprintConfig(
		strings.TrimSpace(release.GetName()),
		release.GetInitPrompt(),
		release.GetPersistentPrompt(),
	).
		WithPlanPrompt(release.GetPlanPrompt()).
		WithResultPrompt(release.GetResultPrompt()).
		WithAIDOptions(aiforge.NewYakForgeBlueprintAIDOptionsConfig().WithDisableToolUse(disableTools))
	blueprint, err := config.Build()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build immutable Forge release: %w", err)
	}
	params := make([]*ypb.ExecParamItem, 0, len(release.GetParameters()))
	for _, parameter := range release.GetParameters() {
		params = append(params, &ypb.ExecParamItem{Key: parameter.GetKey(), Value: parameter.GetValue()})
	}
	return config, blueprint, params, nil
}

func executeContextForgeRelease(
	ctx context.Context,
	release *aiv1.ContextForgeRelease,
	userInput string,
	options ...aicommon.ConfigOption,
) (*aiforge.ForgeResult, error) {
	config, blueprint, params, err := buildContextForgeBlueprint(release)
	if err != nil {
		return nil, err
	}
	coordinator, err := blueprint.CreateCoordinatorWithQueryAndParams(ctx, userInput, params, options...)
	if err != nil {
		return nil, err
	}
	if err := coordinator.Run(); err != nil {
		return nil, err
	}
	return config.ForgeResult, nil
}
