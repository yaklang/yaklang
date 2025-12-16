package volcengine

import (
	"errors"
	"io"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type GatewayClient struct {
	config *aispec.AIConfig

	ExtraOptions []aispec.AIConfigOption

	targetUrl string
}

func (g *GatewayClient) GetConfig() *aispec.AIConfig {
	return g.config
}

func (g *GatewayClient) SupportedStructuredStream() bool {
	return false
}

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

var _ aispec.AIClient = (*GatewayClient)(nil)

func (g *GatewayClient) GetModelList() ([]*aispec.ModelMeta, error) {
	return aispec.ListChatModels(g.targetUrl, g.BuildHTTPOptions)
}

func (g *GatewayClient) Chat(s string, function ...any) (string, error) {
	opts := []aispec.ChatBaseOption{
		aispec.WithChatBase_Function(function),
		aispec.WithChatBase_PoCOptions(g.BuildHTTPOptions),
		aispec.WithChatBase_StreamHandler(g.config.StreamHandler),
		aispec.WithChatBase_ReasonStreamHandler(g.config.ReasonStreamHandler),
		aispec.WithChatBase_ErrHandler(g.config.HTTPErrorHandler),
		aispec.WithChatBase_ImageRawInstance(g.config.Images...),
		aispec.WithChatBase_EnableThinkingEx(g.config.EnableThinking, g.config.EnableThinkingField, g.config.EnableThinkingValue),
	}
	return aispec.ChatBase(g.targetUrl, g.config.Model, s, opts...)
}

func (g *GatewayClient) ChatStream(s string) (io.Reader, error) {
	return aispec.ChatWithStream(
		g.targetUrl, g.config.Model, s, g.config.HTTPErrorHandler, g.config.StreamHandler, g.BuildHTTPOptions,
		aispec.WithChatBase_EnableThinkingEx(g.config.EnableThinking, g.config.EnableThinkingField, g.config.EnableThinkingValue),
	)
}

func (g *GatewayClient) ExtractData(data string, desc string, fields map[string]any) (map[string]any, error) {
	return aispec.ChatBasedExtractData(g.targetUrl, g.config.Model, data, fields, g.BuildHTTPOptions, g.config.StreamHandler, g.config.ReasonStreamHandler, g.config.HTTPErrorHandler, g.config.Images...)
}

func (g *GatewayClient) newLoadOption(opt ...aispec.AIConfigOption) {
	extra := g.ExtraOptions
	extra = append(extra, opt...)
	config := aispec.NewDefaultAIConfig(extra...)

	log.Debug("load option for volcengine ai")
	g.config = config

	if g.config.Model == "" {
		g.config.Model = "doubao-lite-4k"
	}

	g.targetUrl = aispec.GetBaseURLFromConfig(g.config, "https://ark.cn-beijing.volces.com", "/api/v3/chat/completions")
}

func (g *GatewayClient) LoadOption(opt ...aispec.AIConfigOption) {
	if aispec.EnableNewLoadOption {
		g.newLoadOption(opt...)
		return
	}
	config := aispec.NewDefaultAIConfig(opt...)

	log.Debug("load option for volcengine ai")
	g.config = config

	if g.config.Model == "" {
		g.config.Model = "doubao-lite-4k"
	}

	if config.BaseURL != "" {
		g.targetUrl = config.BaseURL
	} else {
		g.targetUrl = "https://ark.cn-beijing.volces.com/api/v3/chat/completions"
	}
}

func (g *GatewayClient) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	opts := []poc.PocConfigOption{
		poc.WithReplaceAllHttpPacketHeaders(map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer " + g.config.APIKey,
		}),
	}
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
	if g.config == nil {
		return utils.Error("bad config (empty)")
	}
	if g.config.APIKey == "" {
		return errors.New("APIKey is required")
	}
	return nil
}
