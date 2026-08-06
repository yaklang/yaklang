package scannode

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func (b *legionJobBridge) handleAIFocusQuery(ctx context.Context, raw []byte) error {
	var command aiv1.QueryAIFocusCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai focus query command: %w", err)
	}

	ref := aiFocusRefFromQueryCommand(&command)
	if err := validateAIFocusQueryCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIFocusQueryFailed(
			ctx,
			ref,
			"invalid_ai_focus_query_command",
			err.Error(),
		)
	}

	metas := reactloops.GetAllLoopMetadata()
	items := make([]*aiv1.AIFocus, 0, len(metas))
	for _, meta := range metas {
		if meta == nil || meta.IsHidden {
			continue
		}
		items = append(items, &aiv1.AIFocus{
			Name:                strings.TrimSpace(meta.Name),
			Description:         strings.TrimSpace(meta.Description),
			OutputExamplePrompt: strings.TrimSpace(meta.OutputExamplePrompt),
			UsagePrompt:         strings.TrimSpace(meta.UsagePrompt),
			VerboseName:         strings.TrimSpace(meta.VerboseName),
			VerboseNameZh:       strings.TrimSpace(meta.VerboseNameZh),
			DescriptionZh:       strings.TrimSpace(meta.GetDescriptionZh()),
		})
	}
	return b.ensureAIPublisher().PublishAIFocusQueried(ctx, ref, items)
}

func validateAIFocusQueryCommand(nodeID string, command *aiv1.QueryAIFocusCommand) error {
	nodeID = strings.TrimSpace(nodeID)
	switch {
	case command == nil:
		return fmt.Errorf("command is required")
	case command.GetMetadata() == nil:
		return fmt.Errorf("metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("owner_user_id is required")
	default:
		return nil
	}
}

func aiFocusRefFromQueryCommand(command *aiv1.QueryAIFocusCommand) aiFocusCommandRef {
	if command == nil {
		return aiFocusCommandRef{}
	}
	return aiFocusCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}
