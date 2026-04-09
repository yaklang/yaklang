package yakgrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) ListAiModel(ctx context.Context, req *ypb.ListAiModelRequest) (*ypb.ListAiModelResponse, error) {
	if req == nil {
		return nil, utils.Error("request is nil")
	}
	if req.Config == "" {
		return nil, utils.Errorf("list ai failed, config is empty")
	}

	config := &aispec.AIConfig{}
	err := json.Unmarshal([]byte(req.Config), config)
	if err != nil {
		return nil, err
	}
	//if config.APIKey == "" {
	//	return nil, utils.Errorf("list ai failed, config.APIKey is empty")
	//}
	models, err := ai.ListModels(
		aispec.WithAPIKey(config.APIKey),
		aispec.WithType(config.Type),
		aispec.WithBaseURL(config.BaseURL),
		aispec.WithEndpoint(config.Endpoint),
		aispec.WithEnableEndpoint(config.EnableEndpoint),
		aispec.WithExtraHeader(aispec.ExtraHeadersToMap(config.Headers)),
		aispec.WithNoHttps(config.NoHttps),
		aispec.WithDomain(config.Domain),
		aispec.WithProxy(config.Proxy),
	)
	if err != nil {
		return nil, err
	}
	rsp := &ypb.ListAiModelResponse{}
	for _, model := range models {
		rsp.ModelName = append(rsp.ModelName, model.Id)
	}
	return rsp, nil
}

func (s *Server) AIConfigHealthCheck(ctx context.Context, req *ypb.AIConfigHealthCheckRequest) (*ypb.AIConfigHealthCheckResponse, error) {
	if req == nil {
		return nil, utils.Error("request is nil")
	}
	if req.GetConfig() == nil {
		return nil, utils.Error("config is nil")
	}
	if strings.TrimSpace(req.GetContent()) == "" {
		return nil, utils.Error("content is empty")
	}

	providerType := strings.TrimSpace(req.GetConfig().GetType())
	if providerType == "" {
		return nil, utils.Error("config.type is empty")
	}
	if !ai.HaveAI(providerType) {
		return nil, utils.Errorf("unsupported ai type: %s", providerType)
	}

	resp := executeAIConfigHealthCheck(ctx, req.GetConfig(), req.GetContent())
	if !isAIConfigHealthCheckPassed(resp) {
		if recommend := recommendAIHealthCheckConfig(ctx, req.GetConfig(), req.GetContent()); recommend != nil {
			resp.RecommendConfig = recommend
		}
	}
	return sanitizeAIConfigHealthCheckResponse(resp), nil
}

func executeAIConfigHealthCheck(ctx context.Context, config *ypb.ThirdPartyApplicationConfig, content string) *ypb.AIConfigHealthCheckResponse {
	resp := &ypb.AIConfigHealthCheckResponse{}
	start := time.Now()
	var firstByteOnce sync.Once
	markFirstByte := func(reader io.Reader) {
		buffered := bufio.NewReader(reader)
		if _, err := buffered.ReadByte(); err == nil {
			firstByteOnce.Do(func() {
				resp.FirstByteCostMs = time.Since(start).Milliseconds()
			})
			_ = buffered.UnreadByte()
		}
		_, _ = io.Copy(io.Discard, buffered)
	}

	opts := aispec.BuildOptionsFromConfig(&ypb.AIModelConfig{
		Provider: cloneThirdPartyApplicationConfig(config),
	})
	opts = append(opts,
		aispec.WithContext(ctx),
		aispec.WithDisableProviderFallback(true),
		aispec.WithStreamHandler(markFirstByte),
		aispec.WithReasonStreamHandler(markFirstByte),
		aispec.WithRawHTTPRequestResponseCallback(func(requestBytes []byte, responseHeaderBytes []byte, bodyPreview []byte) {
			resp.RawRequest = string(requestBytes)
			resp.RawResponse = string(responseHeaderBytes) + string(bodyPreview)
			resp.ResponseStatusCode = int32(lowhttp.GetStatusCodeFromResponse(responseHeaderBytes))
		}),
	)

	result, err := ai.Chat(buildAIHealthCheckPrompt(content), opts...)
	resp.TotalCostMs = time.Since(start).Milliseconds()
	if err != nil {
		resp.ErrorMessage = err.Error()
	} else if resp.GetResponseStatusCode() >= 400 {
		resp.ErrorMessage = utils.Errorf("ai health check failed with status code %d", resp.GetResponseStatusCode()).Error()
	} else if _, actionErr := parseAIHealthCheckCallToolAction(result, content); actionErr != nil {
		resp.ErrorMessage = actionErr.Error()
	}
	resp.ResponseContent = result
	resp.Success = resp.GetResponseStatusCode() < 400 && strings.TrimSpace(resp.GetErrorMessage()) == ""
	return resp
}

const aiHealthCheckToolName = "ai_config_health_check"

func buildAIHealthCheckPrompt(content string) string {
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
%s`, aiHealthCheckToolName, aiHealthCheckToolName, content)
}

func parseAIHealthCheckCallToolAction(raw string, fallbackContent string) (*aicommon.Action, error) {
	action, err := aicommon.ExtractAction(raw, "call-tool")
	if err != nil {
		return nil, utils.Wrap(err, "ai health check failed: parse call-tool action")
	}
	if strings.TrimSpace(action.GetString("tool")) != aiHealthCheckToolName {
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
		return nil, utils.Errorf("ai health check failed: parsed call-tool content mismatch, want %q got %q", expectedContent, content)
	}
	if strings.TrimSpace(params.GetString("summary")) == "" {
		return nil, utils.Error("ai health check failed: parsed call-tool summary is empty")
	}
	return action, nil
}

func isAIConfigHealthCheckPassed(resp *ypb.AIConfigHealthCheckResponse) bool {
	if resp == nil {
		return false
	}
	return resp.GetSuccess()
}

func sanitizeAIConfigHealthCheckResponse(resp *ypb.AIConfigHealthCheckResponse) *ypb.AIConfigHealthCheckResponse {
	if resp == nil {
		return nil
	}
	resp.RawRequest = sanitizeAIHealthCheckString(resp.GetRawRequest())
	resp.ResponseContent = sanitizeAIHealthCheckString(resp.GetResponseContent())
	resp.ErrorMessage = sanitizeAIHealthCheckString(resp.GetErrorMessage())
	resp.RawResponse = sanitizeAIHealthCheckString(resp.GetRawResponse())
	return resp
}

func sanitizeAIHealthCheckString(raw string) string {
	if raw == "" {
		return ""
	}
	return utils.EscapeInvalidUTF8Byte([]byte(raw))
}

func recommendAIHealthCheckConfig(ctx context.Context, original *ypb.ThirdPartyApplicationConfig, content string) *ypb.ThirdPartyApplicationConfig {
	return findFirstWorkingAIConfig(ctx, buildRecommendedAIConfigs(original), content)
}

func buildRecommendedAIConfigs(original *ypb.ThirdPartyApplicationConfig) []*ypb.ThirdPartyApplicationConfig {
	if original == nil {
		return nil
	}
	apiTypeCandidates := recommendedAIAPITypes(original.GetAPIType())
	proxyCandidates := []string{strings.TrimSpace(original.GetProxy())}
	if proxyCandidates[0] != "" {
		proxyCandidates = append(proxyCandidates, "")
	}
	seen := make(map[string]struct{})
	var candidates []*ypb.ThirdPartyApplicationConfig
	addCandidate := func(cfg *ypb.ThirdPartyApplicationConfig) {
		if cfg == nil {
			return
		}
		rawKey := strings.TrimSpace(cfg.GetBaseURL()) + "|" + strings.TrimSpace(cfg.GetEndpoint()) + "|" + cfg.GetProxy() + "|" + strings.TrimSpace(cfg.GetAPIType()) + "|" + utils.InterfaceToString(cfg.GetEnableEndpoint())
		if _, ok := seen[rawKey]; ok {
			return
		}
		seen[rawKey] = struct{}{}
		candidates = append(candidates, cfg)
	}

	for _, endpoint := range collectAIEndpoints(original) {
		for _, apiType := range apiTypeCandidates {
			for _, proxyValue := range proxyCandidates {
				cfg := cloneThirdPartyApplicationConfig(original)
				cfg.BaseURL = ""
				cfg.Endpoint = endpoint
				cfg.EnableEndpoint = true
				cfg.Proxy = proxyValue
				cfg.APIType = apiType
				cfg.NoHttps = strings.HasPrefix(strings.ToLower(endpoint), "http://")
				addCandidate(cfg)
			}
		}
	}
	return candidates
}

func collectAIBaseRoots(config *ypb.ThirdPartyApplicationConfig) []string {
	var roots []string
	for _, raw := range []string{
		strings.TrimSpace(config.GetBaseURL()),
		migratedBaseURLForHealthCheck(config),
		baseURLRootFromEndpoint(strings.TrimSpace(config.GetEndpoint())),
	} {
		appendAIBaseRoots(&roots, raw)
	}
	for _, raw := range collectAIEndpoints(config) {
		appendAIBaseRoots(&roots, baseURLRootFromEndpoint(raw))
	}
	rootURL, _ := aiProviderDefaultEndpointForHealthCheck(config.GetType())
	appendAIBaseRoots(&roots, rootURL)
	return compactStrings(roots)
}

func collectAIEndpoints(config *ypb.ThirdPartyApplicationConfig) []string {
	if config == nil {
		return nil
	}
	apiTypeCandidates := recommendedAIAPITypes(config.GetAPIType())
	schemes := []string{"https", "http"}
	if strings.TrimSpace(config.GetEndpoint()) != "" {
		if u, err := url.Parse(strings.TrimSpace(config.GetEndpoint())); err == nil && u.Scheme != "" {
			schemes = append([]string{u.Scheme}, schemes...)
		}
	} else if strings.TrimSpace(config.GetBaseURL()) != "" {
		if u, err := url.Parse(strings.TrimSpace(config.GetBaseURL())); err == nil && u.Scheme != "" {
			schemes = append([]string{u.Scheme}, schemes...)
		}
	}

	var endpoints []string
	for _, raw := range []string{
		strings.TrimSpace(config.GetEndpoint()),
		strings.TrimSpace(config.GetBaseURL()),
		migratedEndpointForHealthCheck(config),
	} {
		if raw == "" {
			continue
		}
		endpoints = append(endpoints, raw)
	}

	for _, root := range collectAIBaseRootsWithoutEndpointExpansion(config) {
		for _, scheme := range schemes {
			rewrittenRoot := rewriteURLScheme(root, scheme)
			for _, apiType := range apiTypeCandidates {
				for _, suffix := range recommendedAISuffixesByAPIType(apiType) {
					endpoint := joinAIBaseURL(rewrittenRoot, suffix)
					if endpoint != "" {
						endpoints = append(endpoints, endpoint)
					}
				}
			}
		}
	}
	return compactStrings(endpoints)
}

func collectAIBaseRootsWithoutEndpointExpansion(config *ypb.ThirdPartyApplicationConfig) []string {
	var roots []string
	for _, raw := range []string{
		strings.TrimSpace(config.GetBaseURL()),
		migratedBaseURLForHealthCheck(config),
		baseURLRootFromEndpoint(strings.TrimSpace(config.GetEndpoint())),
	} {
		appendAIBaseRoots(&roots, raw)
	}
	rootURL, _ := aiProviderDefaultEndpointForHealthCheck(config.GetType())
	appendAIBaseRoots(&roots, rootURL)
	return compactStrings(roots)
}

func appendAIBaseRoots(roots *[]string, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return
	}
	*roots = append(*roots, u.Scheme+"://"+u.Host)
	cleanPath := normalizeAIBasePath(u.Path)
	if cleanPath != "" {
		*roots = append(*roots, u.Scheme+"://"+u.Host+cleanPath)
	}
}

func normalizeAIBasePath(rawPath string) string {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" || rawPath == "/" {
		return ""
	}
	rawPath = strings.TrimRight(rawPath, "/")
	for _, suffix := range []string{
		"/responses",
		"/v1/responses",
		"/api/v1/responses",
		"/chat/completions",
		"/v1/chat/completions",
		"/api/v1/chat/completions",
		"/compatible-mode/v1/chat/completions",
		"/api/v3/chat/completions",
		"/api/paas/v4/chat/completions",
	} {
		if strings.HasSuffix(rawPath, suffix) {
			rawPath = strings.TrimSuffix(rawPath, suffix)
			break
		}
	}
	if rawPath == "" || rawPath == "/" {
		return ""
	}
	return rawPath
}

func baseURLRootFromEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return strings.TrimRight(trimKnownAIEndpointSuffix(endpoint), "/")
	}
	u.Path = trimKnownAIEndpointSuffix(u.Path)
	u.RawPath = ""
	if u.Path == "/" {
		u.Path = ""
	}
	return strings.TrimRight(u.String(), "/")
}

func trimKnownAIEndpointSuffix(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "/")
	for _, suffix := range []string{
		"/responses",
		"/v1/responses",
		"/api/v1/responses",
		"/chat/completions",
		"/v1/chat/completions",
		"/api/v1/chat/completions",
		"/compatible-mode/v1/chat/completions",
		"/api/v3/chat/completions",
		"/api/paas/v4/chat/completions",
	} {
		if strings.HasSuffix(raw, suffix) {
			return strings.TrimSuffix(raw, suffix)
		}
	}
	return raw
}

func joinAIBaseURL(root string, suffix string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	if suffix == "" {
		return strings.TrimRight(root, "/")
	}
	u, err := url.Parse(root)
	if err != nil {
		return ""
	}
	u.Path = path.Join(strings.TrimSuffix(u.Path, "/"), suffix)
	return u.String()
}

func rewriteURLScheme(raw string, scheme string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Scheme = scheme
	return u.String()
}

func migratedBaseURLForHealthCheck(config *ypb.ThirdPartyApplicationConfig) string {
	cfg := cloneThirdPartyApplicationConfig(config)
	if cfg == nil {
		return ""
	}
	return migratedBaseURLForConfig(cfg)
}

func migratedBaseURLForConfig(config *ypb.ThirdPartyApplicationConfig) string {
	if config == nil {
		return ""
	}
	if strings.TrimSpace(config.GetBaseURL()) != "" && !config.GetEnableEndpoint() {
		return strings.TrimSpace(config.GetBaseURL())
	}
	rootURL, defaultURI := aiProviderDefaultEndpointForHealthCheck(config.GetType())
	return strings.TrimSpace(aispec.GetBaseURLRootFromConfig(&aispec.AIConfig{
		Type:           config.GetType(),
		BaseURL:        config.GetBaseURL(),
		Endpoint:       config.GetEndpoint(),
		EnableEndpoint: config.GetEnableEndpoint(),
		Domain:         config.GetDomain(),
		NoHttps:        config.GetNoHttps(),
		APIType:        config.GetAPIType(),
	}, rootURL, defaultURI))
}

func migratedEndpointForHealthCheck(config *ypb.ThirdPartyApplicationConfig) string {
	if config == nil {
		return ""
	}
	if config.GetEnableEndpoint() && strings.TrimSpace(config.GetEndpoint()) != "" {
		return strings.TrimSpace(config.GetEndpoint())
	}
	rootURL, defaultURI := aiProviderDefaultEndpointForHealthCheck(config.GetType())
	return strings.TrimSpace(aispec.GetBaseURLFromConfig(&aispec.AIConfig{
		Type:           config.GetType(),
		BaseURL:        config.GetBaseURL(),
		Endpoint:       config.GetEndpoint(),
		EnableEndpoint: config.GetEnableEndpoint(),
		Domain:         config.GetDomain(),
		NoHttps:        config.GetNoHttps(),
		APIType:        config.GetAPIType(),
	}, rootURL, defaultURI))
}

func cloneThirdPartyApplicationConfig(config *ypb.ThirdPartyApplicationConfig) *ypb.ThirdPartyApplicationConfig {
	if config == nil {
		return nil
	}
	return &ypb.ThirdPartyApplicationConfig{
		Type:           config.GetType(),
		APIKey:         config.GetAPIKey(),
		UserIdentifier: config.GetUserIdentifier(),
		UserSecret:     config.GetUserSecret(),
		Namespace:      config.GetNamespace(),
		Domain:         config.GetDomain(),
		WebhookURL:     config.GetWebhookURL(),
		ExtraParams:    append([]*ypb.KVPair(nil), config.GetExtraParams()...),
		Disabled:       config.GetDisabled(),
		Proxy:          config.GetProxy(),
		NoHttps:        config.GetNoHttps(),
		APIType:        config.GetAPIType(),
		BaseURL:        config.GetBaseURL(),
		Endpoint:       config.GetEndpoint(),
		EnableEndpoint: config.GetEnableEndpoint(),
		Headers:        cloneHTTPHeaders(config.GetHeaders()),
	}
}

func cloneHTTPHeaders(headers []*ypb.KVPair) []*ypb.KVPair {
	if len(headers) == 0 {
		return nil
	}
	cloned := make([]*ypb.KVPair, 0, len(headers))
	for _, header := range headers {
		if header == nil {
			continue
		}
		cloned = append(cloned, &ypb.KVPair{
			Key:   header.GetKey(),
			Value: header.GetValue(),
		})
	}
	return cloned
}

func findFirstWorkingAIConfig(ctx context.Context, candidates []*ypb.ThirdPartyApplicationConfig, content string) *ypb.ThirdPartyApplicationConfig {
	if len(candidates) == 0 {
		return nil
	}

	const maxConcurrency = 50

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan *ypb.ThirdPartyApplicationConfig)
	resultCh := make(chan *ypb.ThirdPartyApplicationConfig, 1)
	var workers sync.WaitGroup

	workerCount := min(maxConcurrency, len(candidates))
	for i := 0; i < workerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case candidate, ok := <-jobs:
					if !ok {
						return
					}
					tryCtx, tryCancel := context.WithTimeout(ctx, 8*time.Second)
					resp := executeAIConfigHealthCheck(tryCtx, candidate, content)
					tryCancel()
					if isAIConfigHealthCheckPassed(resp) {
						select {
						case resultCh <- candidate:
							cancel()
						default:
						}
						return
					}
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, candidate := range candidates {
			select {
			case <-ctx.Done():
				return
			case jobs <- candidate:
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()

	select {
	case candidate := <-resultCh:
		return candidate
	case <-done:
		return nil
	case <-ctx.Done():
		select {
		case candidate := <-resultCh:
			return candidate
		default:
			return nil
		}
	}
}

func aiProviderDefaultEndpointForHealthCheck(providerType string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "deepseek":
		return "https://api.deepseek.com", "/chat/completions"
	case "volcengine":
		return "https://ark.cn-beijing.volces.com", "/api/v3/chat/completions"
	case "tongyi":
		return "https://dashscope.aliyuncs.com", "/compatible-mode/v1/chat/completions"
	case "openrouter":
		return "https://openrouter.ai", "/api/v1/chat/completions"
	case "chatglm":
		return "https://open.bigmodel.cn", "/api/paas/v4/chat/completions"
	case "ollama":
		return "http://127.0.0.1:11434", "/v1/chat/completions"
	case "aibalance":
		return "https://aibalance.yaklang.com", "/v1/chat/completions"
	case "moonshot":
		return "https://api.moonshot.cn", "/v1/chat/completions"
	case "siliconflow":
		return "https://api.siliconflow.cn", "/v1/chat/completions"
	case "openai", "":
		return "https://api.openai.com", "/v1/chat/completions"
	default:
		return "https://api.openai.com", "/v1/chat/completions"
	}
}

func recommendedAISuffixesByAPIType(apiType string) []string {
	if strings.EqualFold(strings.TrimSpace(apiType), string(aispec.ChatBaseInterfaceTypeResponses)) {
		return []string{
			"",
			"/responses",
			"/v1/responses",
			"/api/v1/responses",
		}
	}
	return []string{
		"",
		"/chat/completions",
		"/v1/chat/completions",
		"/api/v1/chat/completions",
		"/compatible-mode/v1/chat/completions",
		"/api/v3/chat/completions",
		"/api/paas/v4/chat/completions",
	}
}

func recommendedAIAPITypes(apiType string) []string {
	apiType = strings.TrimSpace(apiType)
	switch {
	case strings.EqualFold(apiType, string(aispec.ChatBaseInterfaceTypeResponses)):
		return []string{
			string(aispec.ChatBaseInterfaceTypeResponses),
			string(aispec.ChatBaseInterfaceTypeChatCompletions),
		}
	case apiType == "", strings.EqualFold(apiType, string(aispec.ChatBaseInterfaceTypeChatCompletions)):
		return []string{
			string(aispec.ChatBaseInterfaceTypeChatCompletions),
			string(aispec.ChatBaseInterfaceTypeResponses),
		}
	default:
		return compactStrings([]string{
			apiType,
			string(aispec.ChatBaseInterfaceTypeChatCompletions),
			string(aispec.ChatBaseInterfaceTypeResponses),
		})
	}
}

func compactStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
