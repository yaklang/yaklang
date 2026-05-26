package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/log"
	hidsv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/hids/v1"
)

const (
	hidsFileEvidenceKindSingleFile = "single_file_scan"
	hidsFileEvidenceKindDirectory  = "directory_scan"
)

type hidsFileEvidenceCollectInput struct {
	Kind       string
	Path       string
	Recursive  bool
	MaxDepth   int
	MaxEntries int
}

type hidsFileEvidenceCollectCommandRef struct {
	capabilityCommandRef
	hidsFileEvidenceCollectInput
}

func (b *legionJobBridge) handleHIDSFileEvidenceCollect(
	ctx context.Context,
	raw []byte,
) error {
	var command hidsv1.CollectHIDSFileEvidenceCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal hids file evidence collect command: %w", err)
	}

	if b == nil || b.agent == nil || b.agent.capabilityManager == nil {
		return fmt.Errorf("hids file evidence collect capability manager is not configured")
	}
	manager := b.agent.capabilityManager
	currentNodeID := manager.currentNodeID()
	ref := hidsFileEvidenceCollectCommandRefFromCommand(currentNodeID, &command)
	if err := validateHIDSFileEvidenceCollectCommand(currentNodeID, &command); err != nil {
		log.Errorf(
			"invalid hids file evidence collect command: node_id=%s kind=%s path=%s command_id=%s err=%v",
			ref.NodeID,
			ref.Kind,
			ref.Path,
			ref.CommandID,
			err,
		)
		return nil
	}

	result, err := manager.CollectHIDSFileEvidence(ctx, ref.hidsFileEvidenceCollectInput)
	alert := buildHIDSFileEvidenceCollectAlert(ref, result, err)
	if b.capabilityPublisher == nil {
		return nil
	}
	if publishErr := b.capabilityPublisher.PublishAlert(ctx, alert); publishErr != nil {
		return fmt.Errorf("publish hids file evidence collect alert: %w", publishErr)
	}
	return nil
}

func validateHIDSFileEvidenceCollectCommand(
	nodeID string,
	command *hidsv1.CollectHIDSFileEvidenceCommand,
) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("file evidence collect metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("file evidence collect command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("file evidence collect target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != strings.TrimSpace(nodeID):
		return fmt.Errorf("file evidence collect target_node_id mismatch: %s", command.GetTargetNodeId())
	case command.GetCapability() == nil:
		return fmt.Errorf("file evidence collect capability reference is required")
	case !isHIDSCapabilityKey(command.GetCapability().GetCapabilityKey()):
		return fmt.Errorf("file evidence collect capability_key must be hids")
	case !isSupportedHIDSFileEvidenceKind(command.GetKind()):
		return fmt.Errorf("unsupported file evidence kind: %s", command.GetKind())
	case strings.TrimSpace(command.GetPath()) == "":
		return fmt.Errorf("file evidence collect path is required")
	default:
		return nil
	}
}

func hidsFileEvidenceCollectCommandRefFromCommand(
	nodeID string,
	command *hidsv1.CollectHIDSFileEvidenceCommand,
) hidsFileEvidenceCollectCommandRef {
	ref := hidsFileEvidenceCollectCommandRef{
		capabilityCommandRef: capabilityCommandRef{NodeID: nodeID},
	}
	if command == nil {
		return ref
	}
	if command.GetMetadata() != nil {
		ref.CommandID = command.GetMetadata().GetCommandId()
	}
	if targetNodeID := strings.TrimSpace(command.GetTargetNodeId()); targetNodeID != "" {
		ref.NodeID = targetNodeID
	}
	if command.GetCapability() != nil {
		ref.CapabilityKey = command.GetCapability().GetCapabilityKey()
		ref.SpecVersion = normalizeCapabilitySpecVersion(command.GetCapability().GetSpecVersion())
	}
	ref.Kind = strings.TrimSpace(command.GetKind())
	ref.Path = strings.TrimSpace(command.GetPath())
	ref.Recursive = command.GetRecursive()
	ref.MaxDepth = int(command.GetMaxDepth())
	ref.MaxEntries = int(command.GetMaxEntries())
	return ref
}

func isSupportedHIDSFileEvidenceKind(value string) bool {
	switch strings.TrimSpace(value) {
	case hidsFileEvidenceKindSingleFile, hidsFileEvidenceKindDirectory:
		return true
	default:
		return false
	}
}

func buildHIDSFileEvidenceCollectAlert(
	ref hidsFileEvidenceCollectCommandRef,
	result map[string]any,
	scanErr error,
) CapabilityRuntimeAlert {
	observedAt := time.Now().UTC()
	severity := "low"
	title := "文件证据扫描"
	detail := map[string]any{
		"rule_id":          "manual.file_evidence_scan",
		"match_event_type": "file.change",
		"rule_description": "Manual file evidence scan",
		"event": map[string]any{
			"source": "manual.file_evidence_scan",
			"file": map[string]any{
				"path":      ref.Path,
				"operation": "SCAN",
			},
		},
	}
	if len(result) > 0 {
		detail["evidence_results"] = []map[string]any{result}
		if scanHasFindings(result) {
			severity = "high"
		}
	}
	if scanErr != nil {
		title = "文件证据扫描失败"
		severity = "medium"
		detail["evidence_errors"] = []map[string]any{{
			"kind":   ref.Kind,
			"target": ref.Path,
			"error":  scanErr.Error(),
		}}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		raw = []byte(`{"rule_id":"manual.file_evidence_scan","match_event_type":"file.change"}`)
	}
	return CapabilityRuntimeAlert{
		CapabilityKey: stringOrDefault(ref.CapabilityKey, "hids"),
		SpecVersion:   normalizeCapabilitySpecVersion(ref.SpecVersion),
		RuleID:        "manual.file_evidence_scan",
		Severity:      severity,
		Title:         title,
		DetailJSON:    raw,
		ObservedAt:    observedAt,
	}
}

func scanHasFindings(result map[string]any) bool {
	scan, ok := result["scan"].(map[string]any)
	if !ok {
		return false
	}
	if count, ok := scan["finding_count"].(int); ok && count > 0 {
		return true
	}
	if count, ok := scan["finding_count"].(float64); ok && count > 0 {
		return true
	}
	findings, ok := scan["findings"].([]any)
	return ok && len(findings) > 0
}
