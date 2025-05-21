package openai

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
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
	Functions  []any
	Parameters aispec.Parameters
}

type Session struct {
	client   *Client
	messages []aispec.ChatDetail
}

func NewSession(opt ...ConfigOption) *Session {
	return &Session{
		client:   NewOpenAIClient(opt...),
		messages: make([]aispec.ChatDetail, 0),
	}
}

func (s *Session) Chat(message aispec.ChatDetail, opts ...ConfigOption) (aispec.ChatDetails, error) {
	c := NewRawOpenAIClient(opts...)

	// if the message is a tool call, and the tool call ID is not set, set it to the latest tool call ID
	if message.Role == "tool" && message.ToolCallID == "" && len(s.messages) > 0 {
		latestMessage := s.messages[len(s.messages)-1]
		for _, toolCall := range latestMessage.ToolCalls {
			if toolCall == nil || toolCall.ID == "" {
				continue
			}
			message.ToolCallID = toolCall.ID
			break
		}
	}

	s.messages = append(s.messages, message)

	choices, err := s.client.ChatEx(s.messages, c.Functions...)
	details := lo.Map(choices, func(c aispec.ChatChoice, _ int) aispec.ChatDetail {
		return c.Message
	})

	s.messages = append(s.messages, details...)

	return details, err
}

func NewRawOpenAIClient(opts ...ConfigOption) *Client {
	c := &Client{
		Functions: make([]any, 0),
		Parameters: aispec.Parameters{
			Type:       "object",
			Properties: make(map[string]aispec.Property),
		},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func NewOpenAIClient(opts ...ConfigOption) *Client {
	c := &Client{
		Functions: make([]any, 0),
		Parameters: aispec.Parameters{
			Type:       "object",
			Properties: make(map[string]aispec.Property),
		},
	}
	for _, o := range opts {
		o(c)
	}
	config := &aispec.AIConfig{}
	err := consts.GetThirdPartyApplicationConfig("openai", config)
	if err != nil {
		log.Debug(err)
	}
	if config.APIKey != "" && c.APIKey == "" {
		verbose := "sk-...."
		if len(config.APIKey) > 10 {
			verbose = config.APIKey[:10] + "..."
		}
		log.OnceInfoLog("ai-config-apikey", "use openai apikey config: %v", verbose)
		c.APIKey = config.APIKey
	}
	if model := config.Model; model != "" && c.ChatModel == "" {
		c.ChatModel = model
	}
	if domain := config.Domain; domain != "" && c.Domain == "" {
		c.Domain = utils.ExtractHostPort(domain)
	}
	if proxy := config.Proxy; proxy != "" && c.Proxy == "" {
		log.OnceInfoLog("ai-config-proxy", "use openai apikey config: %v", proxy)
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

func (c *Client) ChatEx(messages []aispec.ChatDetail, funcs ...any) ([]aispec.ChatChoice, error) {
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
	c.Functions = append(c.Functions, funcs...)

	chatMessage := aispec.NewChatMessage(chatModel, messages, c.Functions...)
	raw, err := json.Marshal(chatMessage)
	if err != nil {
		return nil, err
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
		return nil, utils.Wrapf(err, "OpenAI Chat failed: http error")
	}
	rspRaw := lowhttp.GetHTTPPacketBody(rsp.RawPacket)
	var comp aispec.ChatCompletion
	err = json.Unmarshal(rspRaw, &comp)
	if err != nil {
		log.Errorf("OpenAI Chat Error: unmarshal completion failed: %#v", string(rspRaw))
		return nil, utils.Wrapf(err, "unmarshal completion failed")
	}

	if comp.Error != nil && comp.Error.Message != "" {
		errorMsg := comp.Error.Message
		if strings.Contains(errorMsg, "increase your rate limit") {
			log.Infof("reach rate limit... sleep 7s")
			time.Sleep(7 * time.Second) // 20 / min
		}
		return nil, utils.Errorf("OpenAI Chat Error: %s", errorMsg)
	}

	return comp.Choices, nil
}

func (c *Client) Chat(data string, funcs ...any) (string, error) {
	choices, err := c.ChatEx([]aispec.ChatDetail{
		{
			Role:    "user",
			Content: data,
		},
	}, funcs...)
	if err != nil {
		return "", err
	}

	return aispec.DetailsToString(lo.Map(choices, func(c aispec.ChatChoice, index int) aispec.ChatDetail { return c.Message })), nil
}
