package openai

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type Client struct {
	Proxy        string
	APIKey       string
	Organization string
	ChatModel    string

	// Role in Org! public model, the role is user
	Role   string
	Domain string

	// function call
	Parameters Parameters
}

func NewOpenAIClient(opt ...ConfigOption) *Client {
	c := &Client{
		Parameters: Parameters{
			Type:       "object",
			Properties: make(map[string]Property),
		},
	}
	for _, o := range opt {
		o(c)
	}
	config := consts.GetThirdPartyApplicationConfig("openai")
	if config.APIKey != "" && c.APIKey == "" {
		verbose := "sk-...."
		if len(config.APIKey) > 10 {
			verbose = config.APIKey[:10] + "..."
		}
		log.Infof("use yakit config: %v", verbose)
		c.APIKey = config.APIKey
	}
	if model := config.GetExtraParam("model"); model != "" && c.ChatModel == "" {
		c.ChatModel = model
	}
	if domain := config.GetExtraParam("domain"); domain != "" && c.Domain == "" {
		c.Domain = domain
	}
	if proxy := config.GetExtraParam("proxy"); proxy != "" && c.Proxy == "" {
		log.Infof("use yakit config ai proxy: %v", proxy)
		c.Proxy = proxy
	}

	if c.APIKey == "" {
		log.Warn("openai api key is empty")
	}
	return c
}

func (c *Client) TranslateToChinese(data string) (string, error) {
	prompt := fmt.Sprintf(`把下面内容翻译成中文并放在JSON中（以result存结果）:\n%v`, strconv.Quote(data))
	results, err := c.Chat(prompt)
	if err != nil {
		return "", err
	}
	transData, _ := jsonextractor.ExtractJSONWithRaw(results)
	if len(transData) > 0 {
		raw := jsonextractor.FixJson([]byte(transData[0]))
		data := make(map[string]string)
		err := json.Unmarshal(raw, &data)
		if err != nil {
			return "", err
		}
		return utils.MapGetString2(data, "result"), nil
	}
	return strings.Trim(results, "\r\n \v\f\""), nil
}

func (c *Client) Chat(data string, funcs ...Function) (string, error) {
	chatModel := c.ChatModel
	if chatModel == "" {
		chatModel = "gpt-3.5-turbo"
	}
	role := c.Role
	if role == "" {
		role = "user"
	}
	domain := "api.openai.com"
	if c.Domain != "" {
		domain = c.Domain
	}
	chatMessage := NewChatMessage(chatModel, []ChatDetail{
		{
			Role:    role,
			Content: data,
		},
	}, funcs...)
	raw, err := json.Marshal(chatMessage)
	if err != nil {
		return "", err
	}

	rsp, _, err := poc.DoPOST(
		fmt.Sprintf("https://%v/v1/chat/completions", domain),
		poc.WithReplaceHttpPacketHeader("Authorization", fmt.Sprintf("Bearer %v", c.APIKey)),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithReplaceHttpPacketBody(raw, false),
		poc.WithProxy(c.Proxy),
		poc.WithConnPool(true),
	)
	if err != nil {
		return "", err
	}
	rspRaw := lowhttp.GetHTTPPacketBody(rsp.RawPacket)
	var comp ChatCompletion
	err = json.Unmarshal(rspRaw, &comp)
	if err != nil {
		spew.Dump(rspRaw)
		return "", utils.Errorf("unmarshal completion failed: %s", err)
	}

	if len(comp.Choices) <= 0 {
		println(string(rspRaw))
		if strings.Contains(string(rspRaw), "increase your rate limit") {
			log.Infof("reach rate limit... sleep 7s")
			time.Sleep(7 * time.Second) // 20 / min
		}
		return "", utils.Errorf("cannot chat... sorry")
	}
	var list []string

	if len(funcs) > 0 {
		list = funk.Map(comp.Choices, func(c ChatChoice) string {
			return c.Message.FunctionCall.Arguments
		}).([]string)
		list = utils.StringArrayFilterEmpty(list)
		if len(list) > 0 {
			return strings.TrimSpace(list[0]), nil
		}
	}

	list = funk.Map(comp.Choices, func(c ChatChoice) string {
		return c.Message.Content
	}).([]string)
	list = utils.StringArrayFilterEmpty(list)
	return strings.Join(list, "\n\n"), nil
}
