package ollama

import (
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"io"
)

type GatewayClient struct {
	config *aispec.AIConfig

	targetUrl string
}

func (g *GatewayClient) Chat(s string, function ...aispec.Function) (string, error) {
	return aispec.ChatBase(g.targetUrl, g.config.Model, s, function, g.BuildHTTPOptions, g.config.StreamHandler)
}

func (g *GatewayClient) ChatEx(details []aispec.ChatDetail, function ...aispec.Function) ([]aispec.ChatChoice, error) {
	return aispec.ChatExBase(g.targetUrl, g.config.Model, details, function, g.BuildHTTPOptions, g.config.StreamHandler)
}

func (g *GatewayClient) ChatStream(s string) (io.Reader, error) {
	return aispec.ChatWithStream(g.targetUrl, g.config.Model, s, g.config.HTTPErrorHandler, g.BuildHTTPOptions)
}

func (g *GatewayClient) ExtractData(msg string, desc string, fields map[string]any) (map[string]any, error) {
	return aispec.ChatBasedExtractData(g.targetUrl, g.config.Model, msg, fields, g.BuildHTTPOptions, g.config.StreamHandler)
}

func (g *GatewayClient) LoadOption(opt ...aispec.AIConfigOption) {
	config := aispec.NewDefaultAIConfig(opt...)
	g.config = config

	if g.config.Model == "" {
		g.config.Model = "deepseek-chat"
	}

	if config.BaseURL != "" {
		g.targetUrl = config.BaseURL
	} else if config.Domain != "" {
		if config.NoHttps {
			g.targetUrl = "http://" + config.Domain + "/chat/completions"
		} else {
			g.targetUrl = "https://" + config.Domain + "/chat/completions"
		}
	} else {
		g.targetUrl = "https://api.deepseek.com/chat/completions"
	}
}

func (g *GatewayClient) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GatewayClient) CheckValid() error {
	//TODO implement me
	panic("implement me")
}

var _ aispec.AIClient = (*GatewayClient)(nil)

func (g *GatewayClient) SupportedStructuredStream() bool { return false }

func (g *GatewayClient) StructuredStream(s string, function ...aispec.Function) (chan *aispec.StructuredData, error) {
	return nil, aispec.ErrUnsupportedMethod
}
