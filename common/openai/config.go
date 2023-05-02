package openai

import (
	"os"
	"path/filepath"
	"strings"
	"yaklang/common/consts"
	"yaklang/common/log"
)

type ConfigOption func(client *Client)

func WithProxy(i string) ConfigOption {
	return func(client *Client) {
		client.Proxy = i
	}
}

func WithAPIKey(i string) ConfigOption {
	return func(client *Client) {
		client.APIKey = i
	}
}

func WithAPIKeyFromYakitHome() ConfigOption {
	return func(client *Client) {
		var raw, err = os.ReadFile(filepath.Join(consts.GetDefaultYakitBaseDir(), "openai-key.txt"))
		if err != nil {
			log.Warnf("cannot find openai-key.txt in %s", consts.GetDefaultYakitProjectsDir())
			return
		}
		client.APIKey = strings.TrimSpace(string(raw))
	}
}

func WithModel(i string) ConfigOption {
	return func(client *Client) {
		client.ChatModel = i
	}
}

func WithDomain(i string) ConfigOption {
	return func(client *Client) {
		client.Domain = i
	}
}

func WithYakProxy() ConfigOption {
	return func(client *Client) {
		client.Domain = "api.ai.yaklang.com"
	}
}
