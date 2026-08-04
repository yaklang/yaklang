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

func (b *legionJobBridge) handleHIDSDesiredSpecDryRun(
	ctx context.Context,
	raw []byte,
) error {
	var command hidsv1.HIDSDesiredSpecDryRunCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal hids desired spec dry-run command: %w", err)
	}

	currentNodeID := b.agent.node.CurrentNodeID()
	ref := hidsDesiredSpecDryRunCommandRefFromCommand(currentNodeID, &command)
	if err := validateHIDSDesiredSpecDryRunCommand(currentNodeID, &command); err != nil {
		return b.hidsDryRunPublisher.PublishDesiredSpecDryRunResult(
			ctx,
			ref,
			newHIDSDesiredSpecDryRunFailureResult(ref, "invalid_dry_run_command", err.Error()),
		)
	}

	result, err := b.agent.capabilityManager.DryRun(CapabilityApplyInput{
		CapabilityKey:   command.GetCapability().GetCapabilityKey(),
		SpecVersion:     command.GetCapability().GetSpecVersion(),
		DesiredSpecJSON: cloneBytes(command.GetDesiredSpecJson()),
	})
	if err != nil {
		code, message := mapCapabilityApplyError(err)
		log.Errorf(
			"hids desired spec dry-run failed: node_id=%s capability=%s spec_version=%s command_id=%s code=%s err=%v",
			ref.NodeID,
			ref.CapabilityKey,
			ref.SpecVersion,
			ref.CommandID,
			code,
			err,
		)
		result = newHIDSDesiredSpecDryRunFailureResult(ref, code, message)
	}
	return b.hidsDryRunPublisher.PublishDesiredSpecDryRunResult(
		b.agent.node.GetRootContext(),
		ref,
		result,
	)
}

func validateHIDSDesiredSpecDryRunCommand(
	nodeID string,
	command *hidsv1.HIDSDesiredSpecDryRunCommand,
) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("dry-run metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("dry-run command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("dry-run target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != strings.TrimSpace(nodeID):
		return fmt.Errorf("dry-run target_node_id mismatch: %s", command.GetTargetNodeId())
	case command.GetCapability() == nil:
		return fmt.Errorf("dry-run capability reference is required")
	case !isHIDSCapabilityKey(command.GetCapability().GetCapabilityKey()):
		return fmt.Errorf("dry-run capability_key must be hids")
	default:
		return nil
	}
}

func hidsDesiredSpecDryRunCommandRefFromCommand(
	nodeID string,
	command *hidsv1.HIDSDesiredSpecDryRunCommand,
) capabilityCommandRef {
	ref := capabilityCommandRef{NodeID: nodeID}
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
	return ref
}

func newHIDSDesiredSpecDryRunFailureResult(
	ref capabilityCommandRef,
	errorCode string,
	errorMessage string,
) CapabilityDryRunResult {
	message := "hids desired spec dry-run failed"
	errorMessage = strings.TrimSpace(errorMessage)
	if errorMessage == "" {
		errorMessage = message
	}
	return CapabilityDryRunResult{
		CapabilityKey: firstNonEmptyHIDSDryRunText(ref.CapabilityKey, "hids"),
		SpecVersion:   normalizeCapabilitySpecVersion(ref.SpecVersion),
		Status:        capabilityDryRunStatusFailed,
		Message:       message,
		DetailJSON: marshalHIDSDryRunDetailJSON(map[string]any{
			"reason": errorMessage,
		}),
		ErrorCode:    strings.TrimSpace(errorCode),
		ErrorMessage: errorMessage,
		ObservedAt:   time.Now().UTC(),
	}
}

func firstNonEmptyHIDSDryRunText(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func marshalHIDSDryRunDetailJSON(detail map[string]any) []byte {
	if len(detail) == 0 {
		return nil
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return nil
	}
	return raw
}
