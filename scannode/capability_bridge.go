package scannode

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/log"
	capabilityv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/capability/v1"
)

type capabilityCommandRef struct {
	CommandID     string
	NodeID        string
	CapabilityKey string
	SpecVersion   string
}

func (b *legionJobBridge) handleCapabilityApply(
	ctx context.Context,
	raw []byte,
) error {
	var command capabilityv1.ApplyCapabilityCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal capability apply command: %w", err)
	}

	ref := capabilityCommandRefFromCommand(b.agent.node.NodeId, &command)
	if err := validateApplyCapabilityCommand(b.agent.node.NodeId, &command); err != nil {
		return b.capabilityPublisher.PublishFailed(
			ctx,
			ref,
			"invalid_capability_command",
			err.Error(),
		)
	}

	result, err := b.agent.capabilityManager.Apply(CapabilityApplyInput{
		CapabilityKey:   command.GetCapability().GetCapabilityKey(),
		SpecVersion:     command.GetCapability().GetSpecVersion(),
		DesiredSpecJSON: cloneBytes(command.GetDesiredSpecJson()),
	})
	if err == nil {
		return b.capabilityPublisher.PublishStatus(
			b.agent.node.GetRootContext(),
			ref,
			result,
		)
	}

	code, message := mapCapabilityApplyError(err)
	log.Errorf(
		"apply capability failed: node_id=%s capability=%s spec_version=%s command_id=%s code=%s err=%v",
		ref.NodeID,
		ref.CapabilityKey,
		ref.SpecVersion,
		ref.CommandID,
		code,
		err,
	)
	return b.capabilityPublisher.PublishFailed(
		b.agent.node.GetRootContext(),
		ref,
		code,
		message,
	)
}

func validateApplyCapabilityCommand(
	nodeID string,
	command *capabilityv1.ApplyCapabilityCommand,
) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("capability metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("capability command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("capability target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("capability target_node_id mismatch: %s", command.GetTargetNodeId())
	case command.GetCapability() == nil:
		return fmt.Errorf("capability reference is required")
	case strings.TrimSpace(command.GetCapability().GetCapabilityKey()) == "":
		return fmt.Errorf("capability_key is required")
	default:
		return nil
	}
}

func capabilityCommandRefFromCommand(
	nodeID string,
	command *capabilityv1.ApplyCapabilityCommand,
) capabilityCommandRef {
	ref := capabilityCommandRef{NodeID: nodeID}
	if command == nil {
		return ref
	}
	if command.GetMetadata() != nil {
		ref.CommandID = command.GetMetadata().GetCommandId()
	}
	if strings.TrimSpace(command.GetTargetNodeId()) != "" {
		ref.NodeID = strings.TrimSpace(command.GetTargetNodeId())
	}
	if command.GetCapability() != nil {
		ref.CapabilityKey = command.GetCapability().GetCapabilityKey()
		ref.SpecVersion = normalizeCapabilitySpecVersion(
			command.GetCapability().GetSpecVersion(),
		)
	}
	return ref
}

func mapCapabilityApplyError(err error) (string, string) {
	switch {
	case errors.Is(err, ErrInvalidCapabilityKey):
		return "invalid_capability_key", err.Error()
	case errors.Is(err, ErrInvalidCapabilitySpec):
		return "invalid_desired_spec", err.Error()
	case errors.Is(err, ErrInvalidHIDSCapabilitySpec):
		return "invalid_desired_spec", err.Error()
	case errors.Is(err, ErrHIDSCapabilityNotCompiled):
		return "capability_not_compiled", err.Error()
	case errors.Is(err, ErrHIDSCapabilityUnsupportedPlatform):
		return "capability_unsupported_platform", err.Error()
	default:
		return "capability_apply_failed", err.Error()
	}
}
