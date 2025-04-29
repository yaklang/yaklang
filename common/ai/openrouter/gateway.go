package openrouter

import (
	"errors"
	"io"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type GetawayClient struct {
	config *aispec.AIConfig

	targetUrl string
}

func (g *GetawayClient) SupportedStructuredStream() bool {
	return true
}

func (g *GetawayClient) GetModelList() ([]*aispec.ModelMeta, error) {
	return aispec.ListChatModels(g.targetUrl, g.BuildHTTPOptions)
}

func (g *GetawayClient) StructuredStream(s string, function ...aispec.Function) (chan *aispec.StructuredData, error) {
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

var _ aispec.AIClient = (*GetawayClient)(nil)

func (g *GetawayClient) Chat(s string, function ...aispec.Function) (string, error) {
	return aispec.ChatBase(g.targetUrl, g.config.Model,
		s,
		aispec.WithChatBase_Function(function),
		aispec.WithChatBase_PoCOptions(g.BuildHTTPOptions),
		aispec.WithChatBase_StreamHandler(g.config.StreamHandler),
		aispec.WithChatBase_ReasonStreamHandler(g.config.ReasonStreamHandler),
		aispec.WithChatBase_ErrHandler(g.config.HTTPErrorHandler),
	)
}

func (g *GetawayClient) ChatEx(details []aispec.ChatDetail, function ...aispec.Function) ([]aispec.ChatChoice, error) {
	return aispec.ChatExBase(g.targetUrl, g.config.Model, details, function, g.BuildHTTPOptions, g.config.StreamHandler)
}

func (g *GetawayClient) ExtractData(msg string, desc string, fields map[string]any) (map[string]any, error) {
	return aispec.ChatBasedExtractData(g.targetUrl, g.config.Model, msg, fields, g.BuildHTTPOptions, g.config.StreamHandler, g.config.ReasonStreamHandler, g.config.HTTPErrorHandler)
}

func (g *GetawayClient) ChatStream(s string) (io.Reader, error) {
	return aispec.ChatWithStream(g.targetUrl, g.config.Model, s, g.config.HTTPErrorHandler, g.config.StreamHandler, g.BuildHTTPOptions)
}

func (g *GetawayClient) LoadOption(opt ...aispec.AIConfigOption) {
	config := aispec.NewDefaultAIConfig(opt...)
	g.config = config

	if g.config.Model == "" {
		g.config.Model = "qwen/qwq-32b:free"
	}

	if config.BaseURL != "" {
		g.targetUrl = config.BaseURL
	} else if config.Domain != "" {
		if config.NoHttps {
			g.targetUrl = "http://" + config.Domain + "/api/v1/chat/completions"
		} else {
			g.targetUrl = "https://" + config.Domain + "/api/v1/chat/completions"
		}
	} else {
		g.targetUrl = "https://openrouter.ai/api/v1/chat/completions"
	}
}

func (g *GetawayClient) CheckValid() error {
	if g.config.APIKey == "" {
		return errors.New("APIKey is required")
	}
	return nil
}

func (g *GetawayClient) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
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
