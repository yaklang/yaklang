package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	aiSessionRuntimeEventDelta              = "ai.session.delta"
	aiSessionRuntimeEventMessage            = "ai.session.message"
	aiSessionRuntimeEventThought            = "ai.session.thought"
	aiSessionRuntimeEventInteractiveRequest = "ai.session.interactive_request"
	aiSessionRuntimeEventToolCall           = "ai.session.tool_call"
	aiSessionRuntimeEventToolResult         = "ai.session.tool_result"
	maxAISessionAttachmentBytes             = 64 << 10
)

type yakAIEngineRuntimeDriver struct{}

func newYakAIEngineRuntimeDriver() aiSessionRuntimeDriver {
	return yakAIEngineRuntimeDriver{}
}

func (yakAIEngineRuntimeDriver) Bind(
	ctx context.Context,
	binding aiSessionBinding,
	emitter aiSessionRuntimeEmitter,
) (aiSessionRuntimeHandle, error) {
	options, err := buildYakAIEngineOptions(ctx, binding, emitter)
	if err != nil {
		return nil, err
	}
	engine, err := aiengine.NewAIEngine(options...)
	if err != nil {
		return nil, fmt.Errorf("create yak ai engine: %w", err)
	}
	return &yakAIEngineRuntimeHandle{
		engine:  engine,
		emitter: emitter,
		binding: binding,
	}, nil
}

type yakAIEngineRuntimeHandle struct {
	engine  *aiengine.AIEngine
	emitter aiSessionRuntimeEmitter
	binding aiSessionBinding

	sendMu sync.Mutex
	mu     sync.Mutex
	closed bool
}

func (h *yakAIEngineRuntimeHandle) SendInput(ctx context.Context, input aiSessionInput) error {
	if h == nil || h.engine == nil {
		return fmt.Errorf("yak ai engine is not ready")
	}
	content, interactive, syncEvent, options, err := yakAIInputContent(input)
	if err != nil {
		return err
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return fmt.Errorf("yak ai engine is closed")
	}
	h.mu.Unlock()

	if interactive {
		if err := h.engine.SendInteractiveResponse(content); err != nil {
			return fmt.Errorf("send yak ai interactive response: %w", err)
		}
		return nil
	}
	if syncEvent != nil {
		if err := dispatchYakAISyncEvent(h.engine.GetOperator(), syncEvent); err != nil {
			return fmt.Errorf("send yak ai sync event: %w", err)
		}
		return nil
	}

	go h.sendMessage(ctx, content, options...)
	return nil
}

func (h *yakAIEngineRuntimeHandle) AppendContext(ctx context.Context, update aiSessionContextUpdate) error {
	if h == nil || h.engine == nil {
		return fmt.Errorf("yak ai engine is not ready")
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return fmt.Errorf("yak ai engine is closed")
	}
	h.mu.Unlock()

	content, err := renderAISessionContextUpdate(ctx, h.binding, update)
	if err != nil {
		return err
	}
	go h.sendMessage(ctx, content)
	return nil
}

func (h *yakAIEngineRuntimeHandle) Cancel(reason string) {
	if h == nil || h.engine == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	h.mu.Unlock()
	h.engine.Close()
}

func (h *yakAIEngineRuntimeHandle) Close(reason string) {
	if h == nil || h.engine == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	h.mu.Unlock()
	h.engine.Close()
}

func (h *yakAIEngineRuntimeHandle) sendMessage(ctx context.Context, content string, options ...aiengine.AIEngineConfigOption) {
	h.sendMu.Lock()
	defer h.sendMu.Unlock()

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	if err := h.engine.SendMsg(content, options...); err != nil {
		if ctx.Err() != nil {
			return
		}
		detail := mustJSON(map[string]string{
			"runtime": "yak_ai_engine",
		})
		h.emitter.Failed("yak_ai_send_failed", err.Error(), detail)
	}
}

type yakRuntimeOptions struct {
	UseDefaultAIConfig             *bool             `json:"use_default_ai_config"`
	AIService                      string            `json:"ai_service"`
	AIModelName                    string            `json:"ai_model_name"`
	APIKey                         string            `json:"api_key"`
	BaseURL                        string            `json:"base_url"`
	APIType                        string            `json:"api_type"`
	Domain                         string            `json:"domain"`
	Proxy                          string            `json:"proxy"`
	Endpoint                       string            `json:"endpoint"`
	EnableEndpoint                 *bool             `json:"enable_endpoint"`
	NoHTTPS                        *bool             `json:"no_https"`
	Headers                        map[string]string `json:"headers"`
	MaxIteration                   int               `json:"max_iteration"`
	ReActMaxIteration              int64             `json:"react_max_iteration"`
	ReviewPolicy                   string            `json:"review_policy"`
	EnableSystemFileSystemOperator *bool             `json:"enable_system_file_system_operator"`
	DisableToolUse                 *bool             `json:"disable_tool_use"`
	EnableAISearchTool             *bool             `json:"enable_ai_search_tool"`
	DisallowRequireForUserPrompt   *bool             `json:"disallow_require_for_user_prompt"`
	AllowUserInteract              *bool             `json:"allow_user_interact"`
	AllowPlanUserInteract          *bool             `json:"allow_plan_user_interact"`
	DisableToolIntervalReview      *bool             `json:"disable_tool_interval_review"`
	AIReviewRiskControlScore       *float64          `json:"ai_review_risk_control_score"`
	AICallAutoRetry                *int64            `json:"ai_call_auto_retry"`
	AITransactionRetry             *int64            `json:"ai_transaction_retry"`
	TimelineItemLimit              *int64            `json:"timeline_item_limit"`
	AICallTokenLimit               *int64            `json:"ai_call_token_limit"`
	UserInteractLimit              int64             `json:"user_interact_limit"`
	PlanUserInteractMaxCount       int64             `json:"plan_user_interact_max_count"`
	TimelineContentSizeLimit       int64             `json:"timeline_content_size_limit"`
	Focus                          string            `json:"focus"`
	FocusModeLoop                  string            `json:"focus_mode_loop"`
	Workdir                        string            `json:"workdir"`
	Language                       string            `json:"language"`
}

func buildYakAIEngineOptions(
	ctx context.Context,
	binding aiSessionBinding,
	emitter aiSessionRuntimeEmitter,
) ([]aiengine.AIEngineConfigOption, error) {
	runtimeOptions, err := decodeYakRuntimeOptions(binding.RuntimeOptionSnapshotJSON)
	if err != nil {
		return nil, fmt.Errorf("decode runtime options: %w", err)
	}
	providerOptions, err := decodeYakRuntimeOptions(binding.ProviderPolicySnapshotJSON)
	if err != nil {
		return nil, fmt.Errorf("decode provider policy: %w", err)
	}
	options := mergeYakRuntimeOptions(providerOptions, runtimeOptions)

	config := []aiengine.AIEngineConfigOption{
		aiengine.WithContext(ctx),
		aiengine.WithSessionID(binding.Ref.SessionID),
		aiengine.WithOnEvent(func(_ aicommon.AIEngineOperator, event *schema.AiOutputEvent) {
			if event == nil {
				return
			}
			emitter.Emit(classifyYakAIEvent(event), marshalYakAIOutputEvent(event))
		}),
	}
	if options.MaxIteration > 0 {
		config = append(config, aiengine.WithMaxIteration(options.MaxIteration))
	}
	if options.ReActMaxIteration > 0 {
		config = append(config, aiengine.WithMaxIteration(int(options.ReActMaxIteration)))
	}
	if strings.TrimSpace(options.ReviewPolicy) != "" {
		config = append(config, aiengine.WithReviewPolicy(strings.TrimSpace(options.ReviewPolicy)))
	}
	if options.DisableToolUse != nil {
		config = append(config, aiengine.WithDisableToolUse(*options.DisableToolUse))
	}
	if options.EnableAISearchTool != nil {
		config = append(config, aiengine.WithEnableAISearchTool(*options.EnableAISearchTool))
	}
	if options.AllowUserInteract != nil {
		config = append(config, aiengine.WithAllowUserInteract(*options.AllowUserInteract))
	}
	if options.UserInteractLimit > 0 {
		config = append(config, aiengine.WithUserInteractLimit(options.UserInteractLimit))
	}
	if options.TimelineContentSizeLimit > 0 {
		config = append(config, aiengine.WithTimelineContentLimit(int(options.TimelineContentSizeLimit)))
	}
	if extOptions := buildYakAICommonExtOptions(options); len(extOptions) > 0 {
		config = append(config, aiengine.WithExtOptions(extOptions...))
	}
	if strings.TrimSpace(options.Focus) != "" {
		config = append(config, aiengine.WithFocus(strings.TrimSpace(options.Focus)))
	}
	if strings.TrimSpace(options.FocusModeLoop) != "" {
		config = append(config, aiengine.WithFocus(strings.TrimSpace(options.FocusModeLoop)))
	}
	if strings.TrimSpace(options.Workdir) != "" {
		config = append(config, aiengine.WithWorkdir(strings.TrimSpace(options.Workdir)))
	}
	if strings.TrimSpace(options.Language) != "" {
		config = append(config, aiengine.WithLanguage(strings.TrimSpace(options.Language)))
	}
	config, err = appendYakAttachmentOptions(ctx, config, binding)
	if err != nil {
		return nil, err
	}
	if projection := renderCredentialProjection(binding.CredentialRefs); projection != "" {
		config = append(config, aiengine.WithAttachedFileContent(projection))
	}
	callback, err := loadYakAICallback(options)
	if err != nil {
		return nil, err
	}
	if callback != nil {
		config = append(config, aiengine.WithAICallback(callback))
	}
	return config, nil
}

func buildYakAICommonExtOptions(options yakRuntimeOptions) []aicommon.ConfigOption {
	extOptions := make([]aicommon.ConfigOption, 0, 10)
	if options.EnableSystemFileSystemOperator != nil && *options.EnableSystemFileSystemOperator {
		extOptions = append(extOptions, aicommon.WithSystemFileOperator(), aicommon.WithJarOperator())
	}
	if options.DisallowRequireForUserPrompt != nil && *options.DisallowRequireForUserPrompt {
		extOptions = append(extOptions, aicommon.WithDisallowRequireForUserPrompt())
	}
	if options.AllowPlanUserInteract != nil {
		extOptions = append(extOptions, aicommon.WithAllowPlanUserInteract(*options.AllowPlanUserInteract))
	}
	if options.PlanUserInteractMaxCount > 0 {
		extOptions = append(extOptions, aicommon.WithPlanUserInteractMaxCount(options.PlanUserInteractMaxCount))
	}
	if options.AIReviewRiskControlScore != nil {
		extOptions = append(extOptions, aicommon.WithAgreeAIRiskCtrlScore(*options.AIReviewRiskControlScore))
	}
	if options.AICallAutoRetry != nil {
		extOptions = append(extOptions, aicommon.WithAIAutoRetry(*options.AICallAutoRetry))
	}
	if options.AITransactionRetry != nil {
		extOptions = append(extOptions, aicommon.WithAITransactionRetry(*options.AITransactionRetry))
	}
	if options.DisableToolIntervalReview != nil && *options.DisableToolIntervalReview {
		extOptions = append(extOptions, aicommon.WithDisableToolCallerIntervalReview(true))
	}
	if options.AICallTokenLimit != nil {
		extOptions = append(extOptions, aicommon.WithAiCallTokenLimit(*options.AICallTokenLimit))
	}
	return extOptions
}

func loadYakAICallback(options yakRuntimeOptions) (aicommon.AICallbackType, error) {
	aiService := strings.TrimSpace(options.AIService)
	aiModelName := strings.TrimSpace(options.AIModelName)
	if aiService != "" {
		aiConfigOptions := buildYakExplicitAIConfigOptions(options)
		if len(aiConfigOptions) > 0 {
			chat, loadErr := ai.LoadChater(aiService, aiConfigOptions...)
			if loadErr == nil {
				return aicommon.AIChatToAICallbackType(chat), nil
			}
			callback, err := aicommon.GetAIModelCallbackByTierAndProviderAndModel(
				consts.TierIntelligent,
				aiService,
				aiModelName,
			)
			if err == nil {
				return callback, nil
			}
			return nil, fmt.Errorf("load ai service %s: direct=%v tiered=%w", aiService, loadErr, err)
		}

		callback, err := aicommon.GetAIModelCallbackByTierAndProviderAndModel(
			consts.TierIntelligent,
			aiService,
			aiModelName,
		)
		if err == nil {
			return callback, nil
		}

		chat, loadErr := ai.LoadChater(aiService, aiConfigOptions...)
		if loadErr != nil {
			return nil, fmt.Errorf("load ai service %s: tiered=%v fallback=%w", aiService, err, loadErr)
		}
		return aicommon.AIChatToAICallbackType(chat), nil
	}

	if options.UseDefaultAIConfig != nil && *options.UseDefaultAIConfig {
		callback, err := aicommon.GetIntelligentAIModelCallback()
		if err != nil {
			return nil, fmt.Errorf("load default ai config: %w", err)
		}
		return callback, nil
	}

	return nil, nil
}

func buildYakExplicitAIConfigOptions(options yakRuntimeOptions) []aispec.AIConfigOption {
	aiConfigOptions := make([]aispec.AIConfigOption, 0, 8)
	if modelName := strings.TrimSpace(options.AIModelName); modelName != "" {
		aiConfigOptions = append(aiConfigOptions, aispec.WithModel(modelName))
	}
	if apiKey := strings.TrimSpace(options.APIKey); apiKey != "" {
		aiConfigOptions = append(aiConfigOptions, aispec.WithAPIKey(apiKey))
	}
	if baseURL := strings.TrimSpace(options.BaseURL); baseURL != "" {
		aiConfigOptions = append(aiConfigOptions, aispec.WithBaseURL(baseURL))
	}
	if apiType := strings.TrimSpace(options.APIType); apiType != "" {
		aiConfigOptions = append(aiConfigOptions, aispec.WithAPIType(apiType))
	}
	if domain := strings.TrimSpace(options.Domain); domain != "" {
		aiConfigOptions = append(aiConfigOptions, aispec.WithDomain(domain))
	}
	if proxy := strings.TrimSpace(options.Proxy); proxy != "" {
		aiConfigOptions = append(aiConfigOptions, aispec.WithProxy(proxy))
	}
	if endpoint := strings.TrimSpace(options.Endpoint); endpoint != "" {
		aiConfigOptions = append(aiConfigOptions, aispec.WithEndpoint(endpoint))
	}
	if options.EnableEndpoint != nil {
		aiConfigOptions = append(aiConfigOptions, aispec.WithEnableEndpoint(*options.EnableEndpoint))
	}
	if options.NoHTTPS != nil {
		aiConfigOptions = append(aiConfigOptions, aispec.WithNoHttps(*options.NoHTTPS))
	}
	if len(options.Headers) > 0 {
		aiConfigOptions = append(aiConfigOptions, aispec.WithExtraHeader(options.Headers))
	}
	return aiConfigOptions
}

func appendYakAttachmentOptions(
	ctx context.Context,
	config []aiengine.AIEngineConfigOption,
	binding aiSessionBinding,
) ([]aiengine.AIEngineConfigOption, error) {
	if len(binding.Attachments) == 0 {
		return config, nil
	}
	for _, attachment := range binding.Attachments {
		if strings.TrimSpace(attachment.DownloadURL) == "" {
			if strings.TrimSpace(attachment.AttachmentID) != "" {
				return nil, fmt.Errorf("ai attachment %s download_url is required", attachmentIdentity(attachment))
			}
			log.Warnf("skip ai attachment without download url: %s", attachmentIdentity(attachment))
			continue
		}

		content, err := downloadAISessionAttachment(ctx, binding, attachment)
		if err != nil {
			return nil, fmt.Errorf("download ai attachment %s: %w", attachmentIdentity(attachment), err)
		}
		config = append(config, aiengine.WithAttachedFileContent(content))
	}
	return config, nil
}

func downloadAISessionAttachment(
	ctx context.Context,
	binding aiSessionBinding,
	attachment aiSessionAttachmentRef,
) (string, error) {
	if strings.TrimSpace(binding.PlatformBearerToken) == "" {
		return "", fmt.Errorf("node session token is not ready")
	}

	client := binding.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		strings.TrimSpace(attachment.DownloadURL),
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(binding.PlatformBearerToken))

	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 2048))
		if readErr != nil {
			return "", fmt.Errorf("status=%d read_body=%v", response.StatusCode, readErr)
		}
		return "", fmt.Errorf("status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	limited := io.LimitReader(response.Body, maxAISessionAttachmentBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	truncated := len(raw) > maxAISessionAttachmentBytes
	if truncated {
		raw = raw[:maxAISessionAttachmentBytes]
	}
	if !utf8.Valid(raw) {
		return "", fmt.Errorf("attachment content is not valid utf-8")
	}

	return renderAttachmentContent(attachment, string(raw), truncated), nil
}

func renderAttachmentContent(
	attachment aiSessionAttachmentRef,
	content string,
	truncated bool,
) string {
	var builder strings.Builder
	builder.WriteString("AI Session Attachment\n")
	if filename := strings.TrimSpace(attachment.Filename); filename != "" {
		builder.WriteString("Filename: ")
		builder.WriteString(filename)
		builder.WriteString("\n")
	}
	if contentType := strings.TrimSpace(attachment.ContentType); contentType != "" {
		builder.WriteString("Content-Type: ")
		builder.WriteString(contentType)
		builder.WriteString("\n")
	}
	if attachment.SizeBytes > 0 {
		builder.WriteString(fmt.Sprintf("Size: %d bytes\n", attachment.SizeBytes))
	}
	if sha := strings.TrimSpace(attachment.SHA256); sha != "" {
		builder.WriteString("SHA256: ")
		builder.WriteString(sha)
		builder.WriteString("\n")
	}
	builder.WriteString("\n--- Begin Attachment Content ---\n")
	builder.WriteString(content)
	if truncated {
		builder.WriteString("\n\n[attachment content truncated to 65536 bytes]")
	}
	builder.WriteString("\n--- End Attachment Content ---\n")
	return builder.String()
}

func renderCredentialProjection(refs []aiSessionCredentialRef) string {
	if len(refs) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("AI Session Credential References\n")
	builder.WriteString("These are read-only metadata projections. Secret material is not exposed to the runtime.\n")
	for index, ref := range refs {
		builder.WriteString(fmt.Sprintf("\n[%d]\n", index+1))
		builder.WriteString("credential_id: ")
		builder.WriteString(strings.TrimSpace(ref.CredentialID))
		builder.WriteString("\n")
		if credentialType := strings.TrimSpace(ref.CredentialType); credentialType != "" {
			builder.WriteString("credential_type: ")
			builder.WriteString(credentialType)
			builder.WriteString("\n")
		}
		if scope := strings.TrimSpace(ref.Scope); scope != "" {
			builder.WriteString("scope: ")
			builder.WriteString(scope)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func renderAISessionContextUpdate(
	ctx context.Context,
	binding aiSessionBinding,
	update aiSessionContextUpdate,
) (string, error) {
	var sections []string
	for _, attachment := range update.AttachmentRefs {
		if strings.TrimSpace(attachment.DownloadURL) == "" {
			if strings.TrimSpace(attachment.AttachmentID) != "" {
				return "", fmt.Errorf("ai attachment %s download_url is required", attachmentIdentity(attachment))
			}
			continue
		}
		content, err := downloadAISessionAttachment(ctx, binding, attachment)
		if err != nil {
			return "", fmt.Errorf("download ai attachment %s: %w", attachmentIdentity(attachment), err)
		}
		sections = append(sections, content)
	}
	if projection := renderCredentialProjection(update.CredentialRefs); projection != "" {
		sections = append(sections, projection)
	}
	if len(sections) == 0 {
		return "", fmt.Errorf("ai session context update is empty")
	}

	var builder strings.Builder
	builder.WriteString("AI Session Context Update\n")
	if reason := strings.TrimSpace(update.Reason); reason != "" {
		builder.WriteString("Reason: ")
		builder.WriteString(reason)
		builder.WriteString("\n")
	}
	builder.WriteString("Please use the following appended context in subsequent reasoning.\n")
	for _, section := range sections {
		builder.WriteString("\n")
		builder.WriteString(section)
		builder.WriteString("\n")
	}
	return builder.String(), nil
}

func attachmentIdentity(attachment aiSessionAttachmentRef) string {
	if attachmentID := strings.TrimSpace(attachment.AttachmentID); attachmentID != "" {
		return attachmentID
	}
	if filename := strings.TrimSpace(attachment.Filename); filename != "" {
		return filename
	}
	if objectKey := strings.TrimSpace(attachment.ObjectKey); objectKey != "" {
		return objectKey
	}
	return "unknown"
}

func decodeYakRuntimeOptions(raw []byte) (yakRuntimeOptions, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return yakRuntimeOptions{}, nil
	}
	var options yakRuntimeOptions
	if err := json.Unmarshal(raw, &options); err != nil {
		return yakRuntimeOptions{}, err
	}
	return options, nil
}

func mergeYakRuntimeOptions(base yakRuntimeOptions, overlay yakRuntimeOptions) yakRuntimeOptions {
	if overlay.UseDefaultAIConfig != nil {
		base.UseDefaultAIConfig = overlay.UseDefaultAIConfig
	}
	if overlay.AIService != "" {
		base.AIService = overlay.AIService
	}
	if overlay.AIModelName != "" {
		base.AIModelName = overlay.AIModelName
	}
	if overlay.APIType != "" {
		base.APIType = overlay.APIType
	}
	if overlay.Domain != "" {
		base.Domain = overlay.Domain
	}
	if overlay.Proxy != "" {
		base.Proxy = overlay.Proxy
	}
	if overlay.Endpoint != "" {
		base.Endpoint = overlay.Endpoint
	}
	if overlay.EnableEndpoint != nil {
		base.EnableEndpoint = overlay.EnableEndpoint
	}
	if overlay.NoHTTPS != nil {
		base.NoHTTPS = overlay.NoHTTPS
	}
	if len(overlay.Headers) > 0 {
		base.Headers = overlay.Headers
	}
	if overlay.MaxIteration > 0 {
		base.MaxIteration = overlay.MaxIteration
	}
	if overlay.ReActMaxIteration > 0 {
		base.ReActMaxIteration = overlay.ReActMaxIteration
	}
	if overlay.ReviewPolicy != "" {
		base.ReviewPolicy = overlay.ReviewPolicy
	}
	if overlay.EnableSystemFileSystemOperator != nil {
		base.EnableSystemFileSystemOperator = overlay.EnableSystemFileSystemOperator
	}
	if overlay.DisableToolUse != nil {
		base.DisableToolUse = overlay.DisableToolUse
	}
	if overlay.EnableAISearchTool != nil {
		base.EnableAISearchTool = overlay.EnableAISearchTool
	}
	if overlay.DisallowRequireForUserPrompt != nil {
		base.DisallowRequireForUserPrompt = overlay.DisallowRequireForUserPrompt
	}
	if overlay.AllowUserInteract != nil {
		base.AllowUserInteract = overlay.AllowUserInteract
	}
	if overlay.AllowPlanUserInteract != nil {
		base.AllowPlanUserInteract = overlay.AllowPlanUserInteract
	}
	if overlay.DisableToolIntervalReview != nil {
		base.DisableToolIntervalReview = overlay.DisableToolIntervalReview
	}
	if overlay.AIReviewRiskControlScore != nil {
		base.AIReviewRiskControlScore = overlay.AIReviewRiskControlScore
	}
	if overlay.AICallAutoRetry != nil {
		base.AICallAutoRetry = overlay.AICallAutoRetry
	}
	if overlay.AITransactionRetry != nil {
		base.AITransactionRetry = overlay.AITransactionRetry
	}
	if overlay.TimelineItemLimit != nil {
		base.TimelineItemLimit = overlay.TimelineItemLimit
	}
	if overlay.AICallTokenLimit != nil {
		base.AICallTokenLimit = overlay.AICallTokenLimit
	}
	if overlay.UserInteractLimit > 0 {
		base.UserInteractLimit = overlay.UserInteractLimit
	}
	if overlay.PlanUserInteractMaxCount > 0 {
		base.PlanUserInteractMaxCount = overlay.PlanUserInteractMaxCount
	}
	if overlay.TimelineContentSizeLimit > 0 {
		base.TimelineContentSizeLimit = overlay.TimelineContentSizeLimit
	}
	if overlay.Focus != "" {
		base.Focus = overlay.Focus
	}
	if overlay.FocusModeLoop != "" {
		base.FocusModeLoop = overlay.FocusModeLoop
	}
	if overlay.Workdir != "" {
		base.Workdir = overlay.Workdir
	}
	if overlay.Language != "" {
		base.Language = overlay.Language
	}
	return base
}

type yakAISyncEvent struct {
	SyncType      string
	SyncJSONInput string
}

func dispatchYakAISyncEvent(operator aicommon.AIEngineOperator, syncEvent *yakAISyncEvent) error {
	if syncEvent == nil {
		return nil
	}
	if operator == nil {
		return fmt.Errorf("yak ai engine operator is not ready")
	}
	return operator.SendSyncEvent(syncEvent.SyncType, syncEvent.SyncJSONInput)
}

type yakAIInputAttachedResource struct {
	Type  string `json:"type"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

func yakAIInputContent(input aiSessionInput) (string, bool, *yakAISyncEvent, []aiengine.AIEngineConfigOption, error) {
	var payload map[string]any
	if err := json.Unmarshal(input.PayloadJSON, &payload); err != nil {
		return "", false, nil, nil, fmt.Errorf("decode ai session input payload: %w", err)
	}
	inputType := strings.ToLower(strings.TrimSpace(input.InputType))
	switch inputType {
	case "interactive", "interactive_response", "review_response", "user_intervention":
		content := firstNonEmptyString(payload, "interactive_json_input", "response", "content", "message", "text")
		if content == "" {
			content = string(input.PayloadJSON)
		}
		return content, true, nil, nil, nil
	case "sync_event":
		syncType := firstNonEmptyString(payload, "sync_type", "syncType", "type")
		if syncType == "" {
			return "", false, nil, nil, fmt.Errorf("ai session sync event type is required")
		}
		var syncJSONInput string
		switch value := payload["sync_json_input"].(type) {
		case string:
			syncJSONInput = strings.TrimSpace(value)
		case nil:
		default:
			raw, err := json.Marshal(value)
			if err != nil {
				return "", false, nil, nil, fmt.Errorf("marshal ai session sync_json_input: %w", err)
			}
			syncJSONInput = string(raw)
		}
		if syncJSONInput == "" {
			switch value := payload["syncJsonInput"].(type) {
			case string:
				syncJSONInput = strings.TrimSpace(value)
			case nil:
			default:
				raw, err := json.Marshal(value)
				if err != nil {
					return "", false, nil, nil, fmt.Errorf("marshal ai session syncJsonInput: %w", err)
				}
				syncJSONInput = string(raw)
			}
		}
		return "", false, &yakAISyncEvent{
			SyncType:      syncType,
			SyncJSONInput: syncJSONInput,
		}, nil, nil
	default:
		content := firstNonEmptyString(payload, "content", "message", "text", "free_input")
		if content == "" {
			return "", false, nil, nil, fmt.Errorf("ai session message content is required")
		}
		return content, false, nil, yakAIInputAttachedResourceOptions(payload), nil
	}
}

func yakAIInputAttachedResourceOptions(payload map[string]any) []aiengine.AIEngineConfigOption {
	rawValue, ok := payload["attached_resource_info"]
	if !ok {
		rawValue, ok = payload["attachedResourceInfo"]
	}
	if !ok {
		rawValue, ok = payload["AttachedResourceInfo"]
	}
	if !ok || rawValue == nil {
		return nil
	}
	raw, err := json.Marshal(rawValue)
	if err != nil {
		return nil
	}
	var resources []yakAIInputAttachedResource
	if err := json.Unmarshal(raw, &resources); err != nil {
		return nil
	}
	options := make([]aiengine.AIEngineConfigOption, 0, len(resources))
	for _, resource := range resources {
		resourceType := strings.TrimSpace(resource.Type)
		key := strings.TrimSpace(resource.Key)
		value := strings.TrimSpace(resource.Value)
		if resourceType == "" || key == "" || value == "" {
			continue
		}
		options = append(options, aiengine.WithAttachedResource(resourceType, key, value))
	}
	return options
}

func firstNonEmptyString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func classifyYakAIEvent(event *schema.AiOutputEvent) string {
	if event == nil {
		return legionEventAISessionEvent
	}
	if event.IsInteractive() {
		return aiSessionRuntimeEventInteractiveRequest
	}
	switch event.Type {
	case schema.EVENT_TYPE_STREAM:
		return aiSessionRuntimeEventDelta
	case schema.EVENT_TYPE_THOUGHT:
		return aiSessionRuntimeEventThought
	case schema.EVENT_TYPE_RESULT, schema.EVENT_TYPE_SUCCESS_REACT:
		return aiSessionRuntimeEventMessage
	case schema.EVENT_TOOL_CALL_RESULT, schema.EVENT_TOOL_CALL_DONE, schema.EVENT_TOOL_CALL_SUMMARY:
		return aiSessionRuntimeEventToolResult
	case schema.EVENT_TOOL_CALL_START,
		schema.EVENT_TOOL_CALL_STATUS,
		schema.EVENT_TOOL_CALL_DECISION,
		schema.EVENT_TOOL_CALL_ERROR,
		schema.EVENT_TOOL_CALL_USER_CANCEL:
		return aiSessionRuntimeEventToolCall
	default:
		return legionEventAISessionEvent
	}
}

func marshalYakAIOutputEvent(event *schema.AiOutputEvent) []byte {
	if event == nil {
		return nil
	}
	payload := map[string]any{
		"runtime":                "yak_ai_engine",
		"type":                   string(event.Type),
		"node_id":                event.NodeId,
		"is_system":              event.IsSystem,
		"is_stream":              event.IsStream,
		"is_reason":              event.IsReason,
		"is_sync":                event.IsSync,
		"is_json":                event.IsJson,
		"content":                string(event.Content),
		"stream_delta":           string(event.StreamDelta),
		"timestamp":              event.Timestamp,
		"task_index":             event.TaskIndex,
		"task_uuid":              event.TaskUUID,
		"event_uuid":             event.EventUUID,
		"sync_id":                event.SyncID,
		"call_tool_id":           event.CallToolID,
		"content_type":           event.ContentType,
		"ai_service":             event.AIService,
		"ai_model_name":          event.AIModelName,
		"ai_model_verbose_name":  event.AIModelVerboseName,
		"task_semantic_label":    event.TaskSemanticLabel,
		"disable_markdown":       event.DisableMarkdown,
		"emitted_at_unix_millis": time.Now().UTC().UnixMilli(),
	}
	if event.IsJson && json.Valid(event.Content) {
		payload["content_json"] = json.RawMessage(event.Content)
	}
	return mustJSON(payload)
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return raw
}

func logAISessionRuntimePublishError(kind string, sessionID string, err error) {
	log.Errorf("publish ai session runtime %s failed: session_id=%s err=%v", kind, sessionID, err)
}
