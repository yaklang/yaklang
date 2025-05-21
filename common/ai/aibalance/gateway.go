package aibalance

import (
	"errors"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"io"
)

type GatewayClient struct {
	config *aispec.AIConfig

	targetUrl string
}

func (g *GatewayClient) SupportedStructuredStream() bool {
	return true
}

func (g *GatewayClient) GetModelList() ([]*aispec.ModelMeta, error) {
	return aispec.ListChatModels(g.targetUrl, g.BuildHTTPOptions)
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

func (g *GatewayClient) Chat(s string, function ...any) (string, error) {
	return aispec.ChatBase(
		g.targetUrl,
		g.config.Model,
		s,
		aispec.WithChatBase_Function(function),
		aispec.WithChatBase_PoCOptions(g.BuildHTTPOptions),
		aispec.WithChatBase_StreamHandler(g.config.StreamHandler),
		aispec.WithChatBase_ReasonStreamHandler(g.config.ReasonStreamHandler),
		aispec.WithChatBase_ErrHandler(g.config.HTTPErrorHandler),
		aispec.WithChatBase_ImageRawInstance(g.config.Images...),
	)
}

func (g *GatewayClient) ChatEx(details []aispec.ChatDetail, function ...any) ([]aispec.ChatChoice, error) {
	return aispec.ChatExBase(g.targetUrl, g.config.Model, details, function, g.BuildHTTPOptions, g.config.StreamHandler)
}

func (g *GatewayClient) ExtractData(msg string, desc string, fields map[string]any) (map[string]any, error) {
	return aispec.ChatBasedExtractData(
		g.targetUrl,
		g.config.Model, msg, fields,
		g.BuildHTTPOptions, g.config.StreamHandler, g.config.ReasonStreamHandler, g.config.HTTPErrorHandler,
	)
}

func (g *GatewayClient) ChatStream(s string) (io.Reader, error) {
	return aispec.ChatWithStream(
		g.targetUrl, g.config.Model, s, g.config.HTTPErrorHandler, g.config.ReasonStreamHandler,
		g.BuildHTTPOptions,
	)
}

func (g *GatewayClient) LoadOption(opt ...aispec.AIConfigOption) {
	config := aispec.NewDefaultAIConfig(opt...)
	g.config = config

	if g.config.Model == "" {
		g.config.Model = "deepseek-v3"
	}

	if config.BaseURL != "" {
		g.targetUrl = config.BaseURL
	} else if config.Domain != "" {
		if config.NoHttps {
			g.targetUrl = "http://" + config.Domain + "/v1/chat/completions"
		} else {
			g.targetUrl = "https://" + config.Domain + "/v1/chat/completions"
		}
	} else {
		g.targetUrl = "https://aibalance.yaklang.com/v1/chat/completions"
	}
}

func (g *GatewayClient) CheckValid() error {
	if g.config.APIKey == "" {
		return errors.New("APIKey is required")
	}
	return nil
}

func (g *GatewayClient) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	opts := []poc.PocConfigOption{
		poc.WithReplaceAllHttpPacketHeaders(map[string]string{
			"Content-Type":  "application/json; charset=UTF-8",
			"Accept":        "application/json",
			"Authorization": "Bearer " + g.config.APIKey,
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
	return opts, nil
}
