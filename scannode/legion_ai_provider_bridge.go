package scannode

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

const (
	aiProviderHealthCheckToolName    = "ai_config_health_check"
	aiProviderHealthCheckTimeout     = 30 * time.Second
	aiProviderHealthCheckDefaultText = "测试成功"
)

type aiProviderPreviewConfig struct {
	ProviderType   string
	BaseURL        string
	APIType        string
	Domain         string
	Proxy          string
	Endpoint       string
	EnableEndpoint bool
	NoHTTPS        bool
	APIKey         string
	DefaultModel   string
	Headers        map[string]string
}

func (b *legionJobBridge) handleAIProviderModelsList(ctx context.Context, raw []byte) error {
	var command aiv1.ListAIProviderModelsCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai provider models list command: %w", err)
	}

	ref := aiProviderRefFromModelsListCommand(&command)
	if err := validateAIProviderModelsListCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishProviderModelsFailed(
			ctx,
			ref,
			"invalid_ai_provider_models_list_command",
			err.Error(),
		)
	}

	config, err := decodeAIProviderPreviewConfig(command.GetProvider())
	if err != nil {
		return b.ensureAIPublisher().PublishProviderModelsFailed(
			ctx,
			ref,
			"invalid_ai_provider_models_list_config",
			err.Error(),
		)
	}

	items, err := listAIProviderModels(ctx, config)
	if err != nil {
		return b.ensureAIPublisher().PublishProviderModelsFailed(
			ctx,
			ref,
			"ai_provider_models_list_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishProviderModelsListed(ctx, ref, items)
}

func (b *legionJobBridge) handleAIProviderHealthCheck(ctx context.Context, raw []byte) error {
	var command aiv1.HealthCheckAIProviderCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai provider health check command: %w", err)
	}

	ref := aiProviderRefFromHealthCheckCommand(&command)
	if err := validateAIProviderHealthCheckCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishProviderHealthCheckFailed(
			ctx,
			ref,
			"invalid_ai_provider_health_check_command",
			err.Error(),
		)
	}

	config, err := decodeAIProviderPreviewConfig(command.GetProvider())
	if err != nil {
		return b.ensureAIPublisher().PublishProviderHealthCheckFailed(
			ctx,
			ref,
			"invalid_ai_provider_health_check_config",
			err.Error(),
		)
	}

	content := strings.TrimSpace(command.GetContent())
	if content == "" {
		content = aiProviderHealthCheckDefaultText
	}
	result, err := executeAIProviderHealthCheck(ctx, config, content)
	if err != nil {
		return b.ensureAIPublisher().PublishProviderHealthCheckFailed(
			ctx,
			ref,
			"ai_provider_health_check_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishProviderHealthCheckCompleted(ctx, ref, result)
}

func validateAIProviderModelsListCommand(nodeID string, command *aiv1.ListAIProviderModelsCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai provider models list metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai provider models list command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai provider models list target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai provider models list target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai provider models list owner_user_id is required")
	case command.GetProvider() == nil:
		return fmt.Errorf("ai provider models list provider config is required")
	default:
		return nil
	}
}

func validateAIProviderHealthCheckCommand(nodeID string, command *aiv1.HealthCheckAIProviderCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai provider health check metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai provider health check command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai provider health check target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai provider health check target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai provider health check owner_user_id is required")
	case command.GetProvider() == nil:
		return fmt.Errorf("ai provider health check provider config is required")
	default:
		return nil
	}
}

func aiProviderRefFromModelsListCommand(command *aiv1.ListAIProviderModelsCommand) aiProviderCommandRef {
	return aiProviderCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiProviderRefFromHealthCheckCommand(command *aiv1.HealthCheckAIProviderCommand) aiProviderCommandRef {
	return aiProviderCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func decodeAIProviderPreviewConfig(
	snapshot *aiv1.AIProviderPreviewConfigSnapshot,
) (aiProviderPreviewConfig, error) {
	if snapshot == nil {
		return aiProviderPreviewConfig{}, fmt.Errorf("provider config is required")
	}
	providerType := strings.TrimSpace(snapshot.GetProviderType())
	if providerType == "" {
		return aiProviderPreviewConfig{}, fmt.Errorf("provider_type is required")
	}
	if !ai.HaveAI(providerType) {
		return aiProviderPreviewConfig{}, fmt.Errorf("unsupported ai type: %s", providerType)
	}
	return aiProviderPreviewConfig{
		ProviderType:   providerType,
		BaseURL:        strings.TrimSpace(snapshot.GetBaseUrl()),
		APIType:        strings.TrimSpace(snapshot.GetApiType()),
		Domain:         strings.TrimSpace(snapshot.GetDomain()),
		Proxy:          strings.TrimSpace(snapshot.GetProxy()),
		Endpoint:       strings.TrimSpace(snapshot.GetEndpoint()),
		EnableEndpoint: snapshot.GetEnableEndpoint(),
		NoHTTPS:        snapshot.GetNoHttps(),
		APIKey:         strings.TrimSpace(snapshot.GetApiKey()),
		DefaultModel:   strings.TrimSpace(snapshot.GetDefaultModel()),
		Headers:        cloneAIProviderHeaderMap(snapshot.GetHeaders()),
	}, nil
}

func listAIProviderModels(
	ctx context.Context,
	config aiProviderPreviewConfig,
) ([]*aiv1.AIProviderPreviewModel, error) {
	models, err := ai.ListModels(buildAIProviderOptions(ctx, config)...)
	if err != nil {
		return nil, err
	}
	items := make([]*aiv1.AIProviderPreviewModel, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		modelID := strings.TrimSpace(model.Id)
		if modelID == "" {
			continue
		}
		if _, ok := seen[modelID]; ok {
			continue
		}
		seen[modelID] = struct{}{}
		items = append(items, &aiv1.AIProviderPreviewModel{
			ModelId: modelID,
			Label:   modelID,
		})
	}
	return items, nil
}

func executeAIProviderHealthCheck(
	ctx context.Context,
	config aiProviderPreviewConfig,
	content string,
) (*aiv1.AIProviderHealthCheckCompleted, error) {
	if strings.TrimSpace(config.DefaultModel) == "" {
		return nil, fmt.Errorf("default_model is required for health check")
	}
	result := checkAIProviderHealthOnce(ctx, config, content)
	if !result.GetSuccess() {
		if recommendation := recommendAIProviderHealthCheckConfig(ctx, config, content); recommendation != nil {
			result.RecommendConfig = encodeAIProviderPreviewConfig(*recommendation)
		}
	}
	return result, nil
}

func checkAIProviderHealthOnce(
	ctx context.Context,
	config aiProviderPreviewConfig,
	content string,
) *aiv1.AIProviderHealthCheckCompleted {
	result := &aiv1.AIProviderHealthCheckCompleted{}
	start := time.Now()
	var firstByteOnce sync.Once
	markFirstByte := func(reader io.Reader) {
		buffered := bufio.NewReader(reader)
		if _, err := buffered.ReadByte(); err == nil {
			firstByteOnce.Do(func() {
				result.FirstByteCostMs = time.Since(start).Milliseconds()
			})
			_ = buffered.UnreadByte()
		}
		_, _ = io.Copy(io.Discard, buffered)
	}

	options := buildAIProviderOptions(ctx, config)
	options = append(
		options,
		aispec.WithModel(config.DefaultModel),
		aispec.WithDisableProviderFallback(true),
		aispec.WithStreamHandler(markFirstByte),
		aispec.WithReasonStreamHandler(markFirstByte),
		aispec.WithRawHTTPRequestResponseCallback(func(requestBytes []byte, responseHeaderBytes []byte, bodyPreview []byte) {
			result.RawRequest = sanitizeAIProviderDebugString(string(requestBytes))
			result.RawResponse = sanitizeAIProviderDebugString(string(responseHeaderBytes) + string(bodyPreview))
			result.ResponseStatusCode = int32(lowhttp.GetStatusCodeFromResponse(responseHeaderBytes))
		}),
	)

	response, err := ai.Chat(buildAIProviderHealthCheckPrompt(content), options...)
	result.TotalCostMs = time.Since(start).Milliseconds()
	result.ResponseContent = sanitizeAIProviderDebugString(response)
	if err != nil {
		result.ErrorMessage = sanitizeAIProviderDebugString(err.Error())
		return result
	}
	if result.GetResponseStatusCode() >= 400 {
		result.ErrorMessage = sanitizeAIProviderDebugString(
			fmt.Sprintf("ai health check failed with status code %d", result.GetResponseStatusCode()),
		)
		return result
	}
	if _, actionErr := parseAIProviderHealthCheckAction(response, content); actionErr != nil {
		result.ErrorMessage = sanitizeAIProviderDebugString(actionErr.Error())
		return result
	}
	result.Success = true
	return result
}

func buildAIProviderOptions(
	ctx context.Context,
	config aiProviderPreviewConfig,
) []aispec.AIConfigOption {
	options := []aispec.AIConfigOption{
		aispec.WithContext(ctx),
		aispec.WithType(config.ProviderType),
		aispec.WithBaseURL(config.BaseURL),
		aispec.WithEndpoint(config.Endpoint),
		aispec.WithEnableEndpoint(config.EnableEndpoint),
		aispec.WithDomain(config.Domain),
		aispec.WithProxy(config.Proxy),
		aispec.WithNoHttps(config.NoHTTPS),
		aispec.WithAPIKey(config.APIKey),
		aispec.WithAPIType(config.APIType),
	}
	if len(config.Headers) > 0 {
		options = append(options, aispec.WithExtraHeader(cloneAIProviderHeaderMap(config.Headers)))
	}
	return options
}

func buildAIProviderHealthCheckPrompt(content string) string {
	return fmt.Sprintf(`You are running an AI provider health check.
Return only one JSON object in this exact action style:
{"@action":"call-tool","tool":"%s","identifier":"health_check","params":{"content":"user input","summary":"brief summary"}}

Requirements:
1. "@action" must be "call-tool"
2. "tool" must be %q
3. "identifier" must be a short snake_case string
4. "params.content" must exactly equal the user input after trimming leading and trailing whitespace
5. "params.summary" must be a non-empty short summary
6. Do not output markdown, explanations, or any extra text

User input:
%s`, aiProviderHealthCheckToolName, aiProviderHealthCheckToolName, content)
}

func parseAIProviderHealthCheckAction(
	raw string,
	fallbackContent string,
) (*aicommon.Action, error) {
	action, err := aicommon.ExtractAction(raw, "call-tool")
	if err != nil {
		return nil, utils.Wrap(err, "ai health check failed: parse call-tool action")
	}
	if strings.TrimSpace(action.GetString("tool")) != aiProviderHealthCheckToolName {
		return nil, utils.Errorf("ai health check failed: unexpected tool %q", action.GetString("tool"))
	}
	if strings.TrimSpace(action.GetString("identifier")) == "" {
		return nil, utils.Error("ai health check failed: call-tool identifier is empty")
	}
	params := action.GetInvokeParams("params")
	if params == nil {
		return nil, utils.Error("ai health check failed: call-tool params are empty")
	}
	expectedContent := strings.TrimSpace(fallbackContent)
	content := strings.TrimSpace(params.GetString("content"))
	if content == "" {
		return nil, utils.Error("ai health check failed: parsed call-tool content is empty")
	}
	if expectedContent == "" {
		return nil, utils.Error("ai health check failed: original content is empty after trim")
	}
	if content != expectedContent {
		return nil, utils.Errorf(
			"ai health check failed: parsed call-tool content mismatch, want %q got %q",
			expectedContent,
			content,
		)
	}
	if strings.TrimSpace(params.GetString("summary")) == "" {
		return nil, utils.Error("ai health check failed: parsed call-tool summary is empty")
	}
	return action, nil
}

func recommendAIProviderHealthCheckConfig(
	ctx context.Context,
	config aiProviderPreviewConfig,
	content string,
) *aiProviderPreviewConfig {
	for _, candidate := range buildAIProviderHealthCandidates(config) {
		tryCtx, cancel := context.WithTimeout(ctx, aiProviderHealthCheckTimeout)
		result := checkAIProviderHealthOnce(tryCtx, candidate, content)
		cancel()
		if result.GetSuccess() {
			copy := candidate
			return &copy
		}
	}
	return nil
}

func buildAIProviderHealthCandidates(
	config aiProviderPreviewConfig,
) []aiProviderPreviewConfig {
	apiTypes := recommendedAIProviderAPITypes(config.APIType)
	proxies := []string{strings.TrimSpace(config.Proxy)}
	if proxies[0] != "" {
		proxies = append(proxies, "")
	}
	seen := make(map[string]struct{}, len(apiTypes)*len(proxies))
	candidates := make([]aiProviderPreviewConfig, 0, len(apiTypes)*len(proxies))
	for _, apiType := range apiTypes {
		for _, proxyValue := range proxies {
			candidate := config
			candidate.APIType = apiType
			candidate.Proxy = proxyValue
			key := candidate.ProviderType + "|" + candidate.BaseURL + "|" + candidate.Endpoint + "|" + candidate.APIType + "|" + candidate.Proxy
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if candidate.APIType == config.APIType && candidate.Proxy == strings.TrimSpace(config.Proxy) {
				continue
			}
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func recommendedAIProviderAPITypes(apiType string) []string {
	switch strings.TrimSpace(strings.ToLower(apiType)) {
	case "responses":
		return []string{"responses", "chat_completions"}
	case "chat_completions", "":
		return []string{"chat_completions", "responses"}
	default:
		return []string{strings.TrimSpace(apiType), "chat_completions", "responses"}
	}
}

func encodeAIProviderPreviewConfig(
	config aiProviderPreviewConfig,
) *aiv1.AIProviderPreviewConfigSnapshot {
	return &aiv1.AIProviderPreviewConfigSnapshot{
		ProviderType:   strings.TrimSpace(config.ProviderType),
		BaseUrl:        strings.TrimSpace(config.BaseURL),
		ApiType:        strings.TrimSpace(config.APIType),
		Domain:         strings.TrimSpace(config.Domain),
		Proxy:          strings.TrimSpace(config.Proxy),
		Endpoint:       strings.TrimSpace(config.Endpoint),
		EnableEndpoint: config.EnableEndpoint,
		NoHttps:        config.NoHTTPS,
		ApiKey:         strings.TrimSpace(config.APIKey),
		DefaultModel:   strings.TrimSpace(config.DefaultModel),
		Headers:        cloneAIProviderHeaderMap(config.Headers),
	}
}

func cloneAIProviderHeaderMap(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		cloned[trimmed] = value
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}

func sanitizeAIProviderDebugString(raw string) string {
	if raw == "" {
		return ""
	}
	return utils.EscapeInvalidUTF8Byte([]byte(raw))
}
