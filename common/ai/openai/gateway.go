package openai

import (
	"errors"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"io"
)

type GetawayClient struct {
	config *aispec.AIConfig

	targetUrl string
}

func (g *GetawayClient) Chat(s string, function ...aispec.Function) (string, error) {
	return aispec.ChatBase(g.targetUrl, g.config.Model, s, function, g.BuildHTTPOptions, g.config.StreamHandler)
}

func (g *GetawayClient) ChatEx(details []aispec.ChatDetail, function ...aispec.Function) ([]aispec.ChatChoice, error) {
	return aispec.ChatExBase(g.targetUrl, g.config.Model, details, function, g.BuildHTTPOptions, g.config.StreamHandler)

}

func (g *GetawayClient) ExtractData(msg string, desc string, fields map[string]any) (map[string]any, error) {
	return aispec.ChatBasedExtractData(g.targetUrl, g.config.Model, msg, fields, g.BuildHTTPOptions, g.config.StreamHandler)
}

func (g *GetawayClient) ChatStream(s string) (io.Reader, error) {
	return aispec.ChatWithStream(g.targetUrl, g.config.Model, s, g.BuildHTTPOptions)
}

func (g *GetawayClient) LoadOption(opt ...aispec.AIConfigOption) {
	config := aispec.NewDefaultAIConfig(opt...)
	g.config = config

	if g.config.Model == "" {
		g.config.Model = "gpt-3.5-turbo"
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
		g.targetUrl = "https://api.openai.com/v1/chat/completions"
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
	return opts, nil
}
