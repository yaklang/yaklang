package scannode

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/log"
	hidsv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/hids/v1"
)

const (
	hidsCurrentStateCollectProcesses   = "processes"
	hidsCurrentStateCollectConnections = "connections"
)

func (b *legionJobBridge) handleHIDSCurrentStateCollect(
	ctx context.Context,
	raw []byte,
) error {
	var command hidsv1.CollectHIDSCurrentStateCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal hids current-state collect command: %w", err)
	}

	if b == nil || b.agent == nil || b.agent.capabilityManager == nil {
		return fmt.Errorf("hids current-state collect capability manager is not configured")
	}
	manager := b.agent.capabilityManager
	currentNodeID := manager.currentNodeID()
	ref := hidsCurrentStateCollectCommandRefFromCommand(currentNodeID, &command)
	if err := validateHIDSCurrentStateCollectCommand(currentNodeID, &command); err != nil {
		log.Errorf(
			"invalid hids current-state collect command: node_id=%s collect_type=%s command_id=%s err=%v",
			ref.NodeID,
			ref.CollectType,
			ref.CommandID,
			err,
		)
		return nil
	}

	if err := manager.CollectHIDSCurrentState(ctx, ref.CollectType); err != nil {
		log.Errorf(
			"hids current-state collect failed: node_id=%s collect_type=%s command_id=%s err=%v",
			ref.NodeID,
			ref.CollectType,
			ref.CommandID,
			err,
		)
	}
	return nil
}

type hidsCurrentStateCollectCommandRef struct {
	capabilityCommandRef
	CollectType string
}

func validateHIDSCurrentStateCollectCommand(
	nodeID string,
	command *hidsv1.CollectHIDSCurrentStateCommand,
) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("current-state collect metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("current-state collect command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("current-state collect target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != strings.TrimSpace(nodeID):
		return fmt.Errorf("current-state collect target_node_id mismatch: %s", command.GetTargetNodeId())
	case command.GetCapability() == nil:
		return fmt.Errorf("current-state collect capability reference is required")
	case !isHIDSCapabilityKey(command.GetCapability().GetCapabilityKey()):
		return fmt.Errorf("current-state collect capability_key must be hids")
	case !isSupportedHIDSCurrentStateCollectType(command.GetCollectType()):
		return fmt.Errorf("unsupported current-state collect_type: %s", command.GetCollectType())
	default:
		return nil
	}
}

func hidsCurrentStateCollectCommandRefFromCommand(
	nodeID string,
	command *hidsv1.CollectHIDSCurrentStateCommand,
) hidsCurrentStateCollectCommandRef {
	ref := hidsCurrentStateCollectCommandRef{
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
	ref.CollectType = strings.TrimSpace(command.GetCollectType())
	return ref
}

func isSupportedHIDSCurrentStateCollectType(value string) bool {
	switch strings.TrimSpace(value) {
	case hidsCurrentStateCollectProcesses, hidsCurrentStateCollectConnections:
		return true
	default:
		return false
	}
}
