package tongyi

import (
	"errors"
	"io"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type GetawayClient struct {
	config *aispec.AIConfig

	targetUrl string
}

func (g *GetawayClient) SupportedStructuredStream() bool {
	return false
}

func (g *GetawayClient) StructuredStream(s string, function ...aispec.Function) (chan *aispec.StructuredData, error) {
	return nil, utils.Error("unsupported method")
}

var _ aispec.AIClient = (*GetawayClient)(nil)

func (g *GetawayClient) GetModelList() ([]*aispec.ModelMeta, error) {
	return aispec.ListChatModels(g.targetUrl, g.BuildHTTPOptions)
}

func (g *GetawayClient) Chat(s string, function ...aispec.Function) (string, error) {
	return aispec.ChatBase(g.targetUrl, g.config.Model, s, function, g.BuildHTTPOptions, g.config.StreamHandler, g.config.ReasonStreamHandler, g.config.HTTPErrorHandler)
}

func (g *GetawayClient) ChatEx(details []aispec.ChatDetail, function ...aispec.Function) ([]aispec.ChatChoice, error) {
	return aispec.ChatExBase(g.targetUrl, g.config.Model, details, function, g.BuildHTTPOptions, g.config.StreamHandler)
}

func (g *GetawayClient) ChatStream(s string) (io.Reader, error) {
	return aispec.ChatWithStream(g.targetUrl, g.config.Model, s, g.config.HTTPErrorHandler, g.config.StreamHandler, g.BuildHTTPOptions)
}

func (g *GetawayClient) ExtractData(data string, desc string, fields map[string]any) (map[string]any, error) {
	return aispec.ChatBasedExtractData(g.targetUrl, g.config.Model, data, fields, g.BuildHTTPOptions, g.config.StreamHandler, g.config.ReasonStreamHandler, g.config.HTTPErrorHandler)
}

func (g *GetawayClient) LoadOption(opt ...aispec.AIConfigOption) {
	config := aispec.NewDefaultAIConfig(opt...)

	log.Info("load option for tongyi ai")
	g.config = config

	if g.config.Model == "" {
		g.config.Model = "qwen-plus"
	}

	if config.BaseURL != "" {
		g.targetUrl = config.BaseURL
	} else {
		g.targetUrl = "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"
	}
}

func (g *GetawayClient) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
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
	return opts, nil
}

func (g *GetawayClient) CheckValid() error {
	if g.config == nil {
		return utils.Error("bad config (empty)")
	}
	if g.config.APIKey == "" {
		return errors.New("APIKey is required")
	}
	return nil
}
