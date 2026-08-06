package scannode

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func (b *legionJobBridge) handleAIGlobalConfigGet(ctx context.Context, raw []byte) error {
	var command aiv1.GetAIGlobalConfigCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai global config get command: %w", err)
	}

	ref := aiProviderRefFromGlobalConfigGetCommand(&command)
	if err := validateAIGlobalConfigGetCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIGlobalConfigFetchFailed(
			ctx,
			ref,
			"invalid_ai_global_config_get_command",
			err.Error(),
		)
	}

	db := consts.GetGormProfileDatabase()
	if db == nil {
		return b.ensureAIPublisher().PublishAIGlobalConfigFetchFailed(
			ctx,
			ref,
			"ai_global_config_unavailable",
			"database not initialized",
		)
	}

	config, err := yakit.GetAIGlobalConfig(db)
	if err != nil {
		return b.ensureAIPublisher().PublishAIGlobalConfigFetchFailed(
			ctx,
			ref,
			"ai_global_config_get_failed",
			err.Error(),
		)
	}
	if config == nil {
		config = &ypb.AIGlobalConfig{}
	}
	return b.ensureAIPublisher().PublishAIGlobalConfigFetched(
		ctx,
		ref,
		encodeAIGlobalConfigSnapshot(config),
	)
}

func (b *legionJobBridge) handleAIGlobalConfigSet(ctx context.Context, raw []byte) error {
	var command aiv1.SetAIGlobalConfigCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai global config set command: %w", err)
	}

	ref := aiProviderRefFromGlobalConfigSetCommand(&command)
	if err := validateAIGlobalConfigSetCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIGlobalConfigUpdateFailed(
			ctx,
			ref,
			"invalid_ai_global_config_set_command",
			err.Error(),
		)
	}

	db := consts.GetGormProfileDatabase()
	if db == nil {
		return b.ensureAIPublisher().PublishAIGlobalConfigUpdateFailed(
			ctx,
			ref,
			"ai_global_config_unavailable",
			"database not initialized",
		)
	}

	config, err := decodeAIGlobalConfigSnapshot(command.GetConfig())
	if err != nil {
		return b.ensureAIPublisher().PublishAIGlobalConfigUpdateFailed(
			ctx,
			ref,
			"invalid_ai_global_config_snapshot",
			err.Error(),
		)
	}
	normalized, err := yakit.SetAIGlobalConfig(db, config)
	if err != nil {
		return b.ensureAIPublisher().PublishAIGlobalConfigUpdateFailed(
			ctx,
			ref,
			"ai_global_config_set_failed",
			err.Error(),
		)
	}
	if err := yakit.ApplyAIGlobalConfig(db, normalized); err != nil {
		return b.ensureAIPublisher().PublishAIGlobalConfigUpdateFailed(
			ctx,
			ref,
			"ai_global_config_apply_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIGlobalConfigUpdated(
		ctx,
		ref,
		encodeAIGlobalConfigSnapshot(normalized),
	)
}

func validateAIGlobalConfigGetCommand(nodeID string, command *aiv1.GetAIGlobalConfigCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai global config get metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai global config get command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai global config get target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai global config get target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai global config get owner_user_id is required")
	default:
		return nil
	}
}

func validateAIGlobalConfigSetCommand(nodeID string, command *aiv1.SetAIGlobalConfigCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai global config set metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai global config set command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai global config set target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai global config set target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai global config set owner_user_id is required")
	case command.GetConfig() == nil:
		return fmt.Errorf("ai global config set config is required")
	default:
		return nil
	}
}

func aiProviderRefFromGlobalConfigGetCommand(command *aiv1.GetAIGlobalConfigCommand) aiProviderCommandRef {
	return aiProviderCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiProviderRefFromGlobalConfigSetCommand(command *aiv1.SetAIGlobalConfigCommand) aiProviderCommandRef {
	return aiProviderCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func encodeAIGlobalConfigSnapshot(config *ypb.AIGlobalConfig) *aiv1.AIGlobalConfigSnapshot {
	if config == nil {
		return &aiv1.AIGlobalConfigSnapshot{}
	}
	return &aiv1.AIGlobalConfigSnapshot{
		Enabled:           config.GetEnabled(),
		RoutingPolicy:     config.GetRoutingPolicy(),
		DisableFallback:   config.GetDisableFallback(),
		DefaultModelId:    config.GetDefaultModelId(),
		GlobalWeight:      config.GetGlobalWeight(),
		IntelligentModels: encodeAIModelConfigSnapshots(config.GetIntelligentModels()),
		LightweightModels: encodeAIModelConfigSnapshots(config.GetLightweightModels()),
		VisionModels:      encodeAIModelConfigSnapshots(config.GetVisionModels()),
		AiPresetPrompt:    config.GetAIPresetPrompt(),
	}
}

func encodeAIModelConfigSnapshots(models []*ypb.AIModelConfig) []*aiv1.AIModelConfigSnapshot {
	if len(models) == 0 {
		return nil
	}
	result := make([]*aiv1.AIModelConfigSnapshot, 0, len(models))
	for _, model := range models {
		if model == nil {
			continue
		}
		result = append(result, &aiv1.AIModelConfigSnapshot{
			ProviderId:  model.GetProviderId(),
			Provider:    encodeAIThirdPartyApplicationConfigSnapshot(model.GetProvider()),
			ModelName:   model.GetModelName(),
			ExtraParams: encodeAIConfigKVPairs(model.GetExtraParams()),
		})
	}
	return result
}

func encodeAIThirdPartyApplicationConfigSnapshot(
	config *ypb.ThirdPartyApplicationConfig,
) *aiv1.AIThirdPartyApplicationConfigSnapshot {
	if config == nil {
		return nil
	}
	return &aiv1.AIThirdPartyApplicationConfigSnapshot{
		Type:           config.GetType(),
		ApiKey:         config.GetAPIKey(),
		UserIdentifier: config.GetUserIdentifier(),
		UserSecret:     config.GetUserSecret(),
		Namespace:      config.GetNamespace(),
		Domain:         config.GetDomain(),
		WebhookUrl:     config.GetWebhookURL(),
		ExtraParams:    encodeAIConfigKVPairs(config.GetExtraParams()),
		Disabled:       config.GetDisabled(),
		Proxy:          config.GetProxy(),
		NoHttps:        config.GetNoHttps(),
		ApiType:        config.GetAPIType(),
		BaseUrl:        config.GetBaseURL(),
		Endpoint:       config.GetEndpoint(),
		EnableEndpoint: config.GetEnableEndpoint(),
		Headers:        encodeAIConfigKVPairs(config.GetHeaders()),
	}
}

func encodeAIConfigKVPairs(items []*ypb.KVPair) []*aiv1.AIConfigKVPair {
	if len(items) == 0 {
		return nil
	}
	result := make([]*aiv1.AIConfigKVPair, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		result = append(result, &aiv1.AIConfigKVPair{
			Key:   item.GetKey(),
			Value: item.GetValue(),
		})
	}
	return result
}

func decodeAIGlobalConfigSnapshot(snapshot *aiv1.AIGlobalConfigSnapshot) (*ypb.AIGlobalConfig, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("ai global config snapshot is required")
	}
	return &ypb.AIGlobalConfig{
		Enabled:           snapshot.GetEnabled(),
		RoutingPolicy:     snapshot.GetRoutingPolicy(),
		DisableFallback:   snapshot.GetDisableFallback(),
		DefaultModelId:    snapshot.GetDefaultModelId(),
		GlobalWeight:      snapshot.GetGlobalWeight(),
		IntelligentModels: decodeAIModelConfigSnapshots(snapshot.GetIntelligentModels()),
		LightweightModels: decodeAIModelConfigSnapshots(snapshot.GetLightweightModels()),
		VisionModels:      decodeAIModelConfigSnapshots(snapshot.GetVisionModels()),
		AIPresetPrompt:    snapshot.GetAiPresetPrompt(),
	}, nil
}

func decodeAIModelConfigSnapshots(models []*aiv1.AIModelConfigSnapshot) []*ypb.AIModelConfig {
	if len(models) == 0 {
		return nil
	}
	result := make([]*ypb.AIModelConfig, 0, len(models))
	for _, model := range models {
		if model == nil {
			continue
		}
		result = append(result, &ypb.AIModelConfig{
			ProviderId:  model.GetProviderId(),
			Provider:    decodeAIThirdPartyApplicationConfigSnapshot(model.GetProvider()),
			ModelName:   model.GetModelName(),
			ExtraParams: decodeAIConfigKVPairs(model.GetExtraParams()),
		})
	}
	return result
}

func decodeAIThirdPartyApplicationConfigSnapshot(
	config *aiv1.AIThirdPartyApplicationConfigSnapshot,
) *ypb.ThirdPartyApplicationConfig {
	if config == nil {
		return nil
	}
	return &ypb.ThirdPartyApplicationConfig{
		Type:           config.GetType(),
		APIKey:         config.GetApiKey(),
		UserIdentifier: config.GetUserIdentifier(),
		UserSecret:     config.GetUserSecret(),
		Namespace:      config.GetNamespace(),
		Domain:         config.GetDomain(),
		WebhookURL:     config.GetWebhookUrl(),
		ExtraParams:    decodeAIConfigKVPairs(config.GetExtraParams()),
		Disabled:       config.GetDisabled(),
		Proxy:          config.GetProxy(),
		NoHttps:        config.GetNoHttps(),
		APIType:        config.GetApiType(),
		BaseURL:        config.GetBaseUrl(),
		Endpoint:       config.GetEndpoint(),
		EnableEndpoint: config.GetEnableEndpoint(),
		Headers:        decodeAIConfigKVPairs(config.GetHeaders()),
	}
}

func decodeAIConfigKVPairs(items []*aiv1.AIConfigKVPair) []*ypb.KVPair {
	if len(items) == 0 {
		return nil
	}
	result := make([]*ypb.KVPair, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		result = append(result, &ypb.KVPair{
			Key:   item.GetKey(),
			Value: item.GetValue(),
		})
	}
	return result
}
