package yakgrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai"
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
	if resp.GetResponseStatusCode() != 200 {
		if recommend := recommendAIHealthCheckConfig(ctx, req.GetConfig(), req.GetContent()); recommend != nil {
			resp.RecommendConfig = recommend
		}
	}
	return resp, nil
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
		aispec.WithStreamHandler(markFirstByte),
		aispec.WithReasonStreamHandler(markFirstByte),
		aispec.WithRawHTTPRequestResponseCallback(func(requestBytes []byte, responseHeaderBytes []byte, bodyPreview []byte) {
			resp.RawRequest = string(requestBytes)
			resp.RawResponse = string(responseHeaderBytes) + string(bodyPreview)
			resp.ResponseStatusCode = int32(lowhttp.GetStatusCodeFromResponse(responseHeaderBytes))
		}),
	)

	result, err := ai.Chat(content, opts...)
	resp.TotalCostMs = time.Since(start).Milliseconds()
	resp.ResponseContent = result
	if err != nil {
		resp.ErrorMessage = err.Error()
	} else if resp.GetResponseStatusCode() >= 400 {
		resp.ErrorMessage = utils.Errorf("ai health check failed with status code %d", resp.GetResponseStatusCode()).Error()
	}
	return resp
}

func recommendAIHealthCheckConfig(ctx context.Context, original *ypb.ThirdPartyApplicationConfig, content string) *ypb.ThirdPartyApplicationConfig {
	candidates := buildRecommendedAIConfigs(original)
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
					if resp.GetResponseStatusCode() == 200 {
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

func buildRecommendedAIConfigs(original *ypb.ThirdPartyApplicationConfig) []*ypb.ThirdPartyApplicationConfig {
	if original == nil {
		return nil
	}
	apiTypeCandidates := recommendedAIAPITypes(original.GetAPIType())

	proxyCandidates := []string{strings.TrimSpace(original.GetProxy())}
	if proxyCandidates[0] != "" {
		proxyCandidates = append(proxyCandidates, "")
	}

	schemes := []string{"https", "http"}
	if strings.TrimSpace(original.GetBaseURL()) != "" {
		if u, err := url.Parse(strings.TrimSpace(original.GetBaseURL())); err == nil && u.Scheme != "" {
			schemes = append([]string{u.Scheme}, schemes...)
		}
	} else if original.GetNoHttps() {
		schemes = append([]string{"http"}, schemes...)
	}

	baseRoots := collectAIBaseRoots(original)
	seen := make(map[string]struct{})
	var candidates []*ypb.ThirdPartyApplicationConfig
	addCandidate := func(cfg *ypb.ThirdPartyApplicationConfig) {
		if cfg == nil || strings.TrimSpace(cfg.GetBaseURL()) == "" {
			return
		}
		key := cfg.GetBaseURL() + "|" + cfg.GetProxy() + "|" + strings.TrimSpace(cfg.GetAPIType())
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, cfg)
	}

	for _, exact := range []string{strings.TrimSpace(original.GetBaseURL()), migratedBaseURLForHealthCheck(original)} {
		if exact == "" {
			continue
		}
		for _, apiType := range apiTypeCandidates {
			for _, proxyValue := range proxyCandidates {
				cfg := cloneThirdPartyApplicationConfig(original)
				cfg.BaseURL = exact
				cfg.Proxy = proxyValue
				cfg.APIType = apiType
				cfg.NoHttps = strings.HasPrefix(strings.ToLower(exact), "http://")
				addCandidate(cfg)
			}
		}
	}

	for _, root := range baseRoots {
		for _, scheme := range schemes {
			rewrittenRoot := rewriteURLScheme(root, scheme)
			for _, apiType := range apiTypeCandidates {
				for _, suffix := range recommendedAISuffixesByAPIType(apiType) {
					baseURL := joinAIBaseURL(rewrittenRoot, suffix)
					if baseURL == "" {
						continue
					}
					for _, proxyValue := range proxyCandidates {
						cfg := cloneThirdPartyApplicationConfig(original)
						cfg.BaseURL = baseURL
						cfg.Proxy = proxyValue
						cfg.APIType = apiType
						cfg.NoHttps = scheme == "http"
						addCandidate(cfg)
					}
				}
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
	if strings.TrimSpace(config.GetBaseURL()) != "" {
		return strings.TrimSpace(config.GetBaseURL())
	}
	rootURL, defaultURI := aiProviderDefaultEndpointForHealthCheck(config.GetType())
	return strings.TrimSpace(aispec.GetBaseURLFromConfig(&aispec.AIConfig{
		Type:    config.GetType(),
		BaseURL: config.GetBaseURL(),
		Domain:  config.GetDomain(),
		NoHttps: config.GetNoHttps(),
		APIType: config.GetAPIType(),
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
