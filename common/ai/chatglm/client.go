package chatglm

import (
	"errors"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"io"
	"strings"
)

type GLMClient struct {
	config *aispec.AIConfig

	targetUrl string
}

func (g *GLMClient) ChatStream(msg string) (io.Reader, error) {
	return aispec.ChatWithStream(g.targetUrl, g.config.Model, msg, g.BuildHTTPOptions)
}

func (g *GLMClient) LoadOption(opt ...aispec.AIConfigOption) {
	config := aispec.NewDefaultAIConfig(opt...)
	g.config = config

	if g.config.Model == "" {
		g.config.Model = "glm-3-turbo"
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
	return opts, nil
}

func (g *GLMClient) Chat(s string, f ...aispec.Function) (string, error) {
	return aispec.ChatBase(g.targetUrl, g.config.Model, s, f, g.BuildHTTPOptions, g.config.StreamHandler)
}

func (g *GLMClient) ChatEx(details []aispec.ChatDetail, function ...aispec.Function) ([]aispec.ChatChoice, error) {
	return aispec.ChatExBase(g.targetUrl, g.config.Model, details, function, g.BuildHTTPOptions, g.config.StreamHandler)
}

func (g *GLMClient) ExtractData(msg string, desc string, fields map[string]any) (map[string]any, error) {
	return aispec.ChatBasedExtractData(g.targetUrl, g.config.Model, msg, fields, g.BuildHTTPOptions, g.config.StreamHandler)
}
