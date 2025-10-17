package chatglm

import (
	"errors"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type GLMClient struct {
	config *aispec.AIConfig

	targetUrl string
}

func (g *GLMClient) GetModelList() ([]*aispec.ModelMeta, error) {
	return aispec.ListChatModels(g.targetUrl, g.BuildHTTPOptions)
}

func (g *GLMClient) SupportedStructuredStream() bool {
	return true
}

func (g *GLMClient) StructuredStream(s string, function ...any) (chan *aispec.StructuredData, error) {
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

var _ aispec.AIClient = (*GLMClient)(nil)

func (g *GLMClient) ChatStream(msg string) (io.Reader, error) {
	return aispec.ChatWithStream(
		g.targetUrl, g.config.Model, msg, g.config.HTTPErrorHandler,
		g.config.ReasonStreamHandler, g.BuildHTTPOptions,
	)
}

func (g *GLMClient) newLoadOption(opt ...aispec.AIConfigOption) {
	config := aispec.NewDefaultAIConfig(opt...)
	g.config = config

	if g.config.Model == "" {
		g.config.Model = "glm-4-flash"
	}

	g.targetUrl = aispec.GetBaseURLFromConfig(g.config, "https://open.bigmodel.cn", "/api/paas/v4/chat/completions")
}

func (g *GLMClient) LoadOption(opt ...aispec.AIConfigOption) {
	if aispec.EnableNewLoadOption {
		g.newLoadOption(opt...)
		return
	}
	config := aispec.NewDefaultAIConfig(opt...)
	g.config = config

	if g.config.Model == "" {
		g.config.Model = "glm-4-flash"
	}

	if config.BaseURL != "" {
		g.targetUrl = config.BaseURL
	} else if config.Domain != "" {
		if config.NoHttps {
			g.targetUrl = "http://" + config.Domain
		} else {
			g.targetUrl = "https://" + config.Domain
		}
		if !strings.Contains(config.Domain, "/") {
			g.targetUrl += "/api/paas/v4/chat/completions"
		}
	} else {
		g.targetUrl = "https://open.bigmodel.cn/api/paas/v4/chat/completions"
	}
}

func (g *GLMClient) CheckValid() error {
	if g.config.APIKey == "" {
		return errors.New("APIKey is required")
	}
	return nil
}

func (g *GLMClient) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	k, err := generateToken(g.config.APIKey)
	if err != nil {
		return nil, err
	}
	opts := []poc.PocConfigOption{
		poc.WithReplaceAllHttpPacketHeaders(map[string]string{
			"Content-Type":  "application/json; charset=UTF-8",
			"Accept":        "application/json",
			"Authorization": k,
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

func (g *GLMClient) Chat(s string, f ...any) (string, error) {
	return aispec.ChatBase(
		g.targetUrl, g.config.Model, s,
		aispec.WithChatBase_Function(f),
		aispec.WithChatBase_PoCOptions(g.BuildHTTPOptions),
		aispec.WithChatBase_StreamHandler(g.config.StreamHandler),
		aispec.WithChatBase_ReasonStreamHandler(g.config.ReasonStreamHandler),
		aispec.WithChatBase_ErrHandler(g.config.HTTPErrorHandler),
		aispec.WithChatBase_ImageRawInstance(g.config.Images...),
	)
}

func (g *GLMClient) ExtractData(msg string, desc string, fields map[string]any) (map[string]any, error) {
	return aispec.ChatBasedExtractData(g.targetUrl, g.config.Model, msg, fields, g.BuildHTTPOptions, g.config.StreamHandler, g.config.ReasonStreamHandler, g.config.HTTPErrorHandler)
}
