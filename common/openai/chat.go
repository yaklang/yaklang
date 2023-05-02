package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
	"yaklang/common/go-funk"
	"yaklang/common/jsonextractor"
	"yaklang/common/log"
	"yaklang/common/utils"
)

type Client struct {
	Proxy        string
	httpClient   *http.Client
	APIKey       string
	Organization string
	ChatModel    string

	// Role in Org! public model, the role is user
	Role   string
	Domain string
}

func NewOpenAIClient(opt ...ConfigOption) *Client {
	c := &Client{}
	for _, o := range opt {
		o(c)
	}
	if c.httpClient == nil {
		c.httpClient = utils.NewDefaultHTTPClientWithProxy(c.Proxy)
		c.httpClient.Timeout = time.Minute
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
		var data = make(map[string]string)
		err := json.Unmarshal(raw, &data)
		if err != nil {
			return "", err
		}
		return utils.MapGetString2(data, "result"), nil
	}
	return strings.Trim(results, "\r\n \v\f\""), nil
}

func (c *Client) Chat(data string) (string, error) {
	// build body
	var chatModel = c.ChatModel
	if chatModel == "" {
		chatModel = "gpt-3.5-turbo"
	}
	var role = c.Role
	if role == "" {
		role = "user"
	}
	raw, err := json.Marshal(NewChatMessage(chatModel, ChatDetail{
		Role:    role,
		Content: data,
	}))
	if err != nil {
		return "", err
	}
	reader := ioutil.NopCloser(bytes.NewBuffer(raw))

	var domain = "api.openai.com"
	if c.Domain != "" {
		domain = c.Domain
	}

	// build request
	req, err := http.NewRequest("POST", fmt.Sprintf(
		"https://%v/v1/chat/completions",
		domain,
	), reader)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %v", c.APIKey))
	if c.Organization != "" {
		req.Header.Set("OpenAI-Organization", c.Organization)
	}
	req.Header.Set("Content-Type", "application/json")
	rsp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	rspRaw, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return "", err
	}
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
	list := funk.Map(comp.Choices, func(c ChatChoice) string {
		return c.Message.Content
	}).([]string)
	list = utils.StringArrayFilterEmpty(list)
	return strings.Join(list, "\n\n"), nil
}
