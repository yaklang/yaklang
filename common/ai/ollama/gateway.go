package ollama

import (
	"errors"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type GatewayClient struct {
	config *aispec.AIConfig

	targetUrl       string
	useOpenAIFormat bool
}

func (g *GatewayClient) GetModelList() ([]*aispec.ModelMeta, error) {
	return aispec.ListChatModels(g.targetUrl, g.BuildHTTPOptions)
}

func (g *GatewayClient) Chat(s string, function ...any) (string, error) {
	return aispec.ChatBase(g.targetUrl, g.config.Model,
		s,
		aispec.WithChatBase_Function(function),
		aispec.WithChatBase_PoCOptions(g.BuildHTTPOptions),
		aispec.WithChatBase_StreamHandler(g.config.StreamHandler),
		aispec.WithChatBase_ReasonStreamHandler(g.config.ReasonStreamHandler),
		aispec.WithChatBase_ErrHandler(g.config.HTTPErrorHandler),
		aispec.WithChatBase_ImageRawInstance(g.config.Images...),
	)
}

func (g *GatewayClient) ChatStream(s string) (io.Reader, error) {
	return aispec.ChatWithStream(g.targetUrl, g.config.Model, s, g.config.HTTPErrorHandler, g.config.StreamHandler, g.BuildHTTPOptions)
}

func (g *GatewayClient) ExtractData(msg string, desc string, fields map[string]any) (map[string]any, error) {
	return aispec.ChatBasedExtractData(g.targetUrl, g.config.Model, msg, fields, g.BuildHTTPOptions, g.config.StreamHandler, g.config.ReasonStreamHandler, g.config.HTTPErrorHandler)
}

func (g *GatewayClient) LoadOption(opt ...aispec.AIConfigOption) {
	config := aispec.NewDefaultAIConfig(opt...)
	g.config = config

	if g.config.Model == "" {
		g.config.Model = "qwen"
	}

	// 默认使用 OpenAI 兼容 API
	g.useOpenAIFormat = true

	// 根据传入参数，决定使用哪种 API 格式
	if config.BaseURL != "" {
		g.targetUrl = config.BaseURL
		// 如果手动指定了完整的 URL，保留它，但默认判断是否包含 OpenAI 兼容格式
		if !strings.Contains(g.targetUrl, "/v1/chat/completions") && !strings.Contains(g.targetUrl, "/api/chat") {
			// 如果既不包含 /v1/chat/completions 也不包含 /api/chat，则添加 OpenAI 兼容路径
			if !strings.HasSuffix(g.targetUrl, "/") {
				g.targetUrl += "/"
			}
			g.targetUrl += "v1/chat/completions"
		}
	} else {
		g.targetUrl = aispec.GetBaseURLFromConfig(g.config, "http://127.0.0.1:11434", "/v1/chat/completions")
	}

	// 检查是否显式指定了使用原生 API（通过模型名后缀）
	for i := range opt {
		tmpConfig := &aispec.AIConfig{}
		opt[i](tmpConfig)
		if strings.Contains(tmpConfig.Model, "native_api") {
			g.useOpenAIFormat = false
			g.config.Model = strings.TrimSuffix(g.config.Model, "_native_api")

			// 如果指定了使用原生 API，修改 URL
			if strings.Contains(g.targetUrl, "/v1/chat/completions") {
				g.targetUrl = strings.Replace(g.targetUrl, "/v1/chat/completions", "/api/chat", 1)
			}
			break
		}
	}
}

func (g *GatewayClient) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	opts := []poc.PocConfigOption{
		poc.WithReplaceAllHttpPacketHeaders(map[string]string{
			"Content-Type": "application/json; charset=UTF-8",
			"Accept":       "application/json",
		}),
	}
	opts = append(opts, poc.WithTimeout(g.config.Timeout))
	if g.config.Proxy != "" {
		opts = append(opts, poc.WithProxy(g.config.Proxy))
	}
	if g.config.Context != nil {
		opts = append(opts, poc.WithContext(g.config.Context))
	}
	if g.config.Timeout > 0 {
		opts = append(opts, poc.WithConnectTimeout(g.config.Timeout))
	}
	opts = append(opts, poc.WithTimeout(600))
	if g.config.Host != "" {
		opts = append(opts, poc.WithHost(g.config.Host))
	}
	if g.config.Port > 0 {
		opts = append(opts, poc.WithPort(g.config.Port))
	}
	return opts, nil
}

func (g *GatewayClient) CheckValid() error {
	host, port, err := utils.ParseStringToHostPort(g.targetUrl)
	if err != nil {
		return err
	}
	if host == "" || port == 0 {
		return errors.New("invalid target url")
	}
	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		return errors.New("ollama not running")
	}
	return nil
}

var _ aispec.AIClient = (*GatewayClient)(nil)

func (g *GatewayClient) SupportedStructuredStream() bool { return true }

func (g *GatewayClient) StructuredStream(s string, function ...any) (chan *aispec.StructuredData, error) {
	return aispec.StructuredStreamBase(
		g.targetUrl,
		g.config.Model,
		s,
		g.BuildHTTPOptions,
		g.config.StreamHandler,
		g.config.ReasonStreamHandler,
		g.config.HTTPErrorHandler,
	)
}
