package comate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/twofa"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type Client struct {
	config *aispec.AIConfig
}

func (c *Client) GetConfig() *aispec.AIConfig {
	return c.config
}

func (c *Client) GetModelList() ([]*aispec.ModelMeta, error) {
	return nil, nil
}

func (c *Client) SupportedStructuredStream() bool {
	return false
}

func (c *Client) StructuredStream(s string, function ...any) (chan *aispec.StructuredData, error) {
	return nil, utils.Error("unsupported method")
}

var (
	tokenFetchingAddr = `https://aliyun-oss.yaklang.com/thirdparty/comate.txt`
	tokenCached       string
	getTokenMutex     = new(sync.Mutex)
)

// token fetching: https://aliyun-oss.yaklang.com/thirdparty/comate.txt
func getToken(twofakey string) (string, error) {
	getTokenMutex.Lock()
	defer getTokenMutex.Unlock()

	if tokenCached != "" {
		return tokenCached, nil
	}

	rsp, _, err := poc.DoGET(tokenFetchingAddr)
	if err != nil {
		return "", utils.Wrapf(err, "failed to fetch token")
	}
	addr := string(bytes.TrimSpace(rsp.GetBody()))
	if twofakey == "" {
		twofakey = "bairiyishanjin"
	}

	rsp, _, err = poc.DoGET("http://"+addr, poc.WithReplaceHttpPacketHeader("Y-T-Verify-Code", twofa.GetUTCCode(twofakey)))
	if err != nil {
		return "", utils.Wrapf(err, "failed to fetch token")
	}
	token := string(rsp.GetBody())
	tokenCached = token
	go func() {
		time.Sleep(3 * time.Hour)
		tokenCached = ""
	}()
	return token, nil
}

func (c *Client) question(i string) (io.Reader, error) {
	var (
		model string = "ernie-bot"
		sec   string
	)
	// ernie-bot / ernie-bot-pro
	if c.config != nil {
		sec = c.config.APIKey
	}
	token, err := getToken(sec)
	if err != nil {
		return nil, err
	}

	input := map[string]any{
		"userInput": i, "codeInLineContent": "",
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	if c.config != nil {
		if c.config.Model != "" {
			model = c.config.Model
		}
	}

	switch model {
	case "ernie-bot", "ernie-bot-pro":
	default:
		model = "ernie-bot"
	}
	u := `https://comate.baidu.com/openapi/gw/` + model + `/chat/stream`

	var r, w = utils.NewBufPipe(nil)
	go func() {
		defer func() {
			time.Sleep(time.Second * 3)
			w.Close()
		}()
		rsp, _, err := poc.DoPOST(
			u, poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
			poc.WithReplaceHttpPacketHeader("x-access-token", token),
			poc.WithReplaceHttpPacketBody(raw, false),
			poc.WithContext(c.config.Context),
			poc.WithConnectTimeout(c.config.Timeout),
			poc.WithTimeout(600),
			poc.WithBodyStreamReaderHandler(func(r []byte, closer io.ReadCloser) {
				scanner := bufio.NewScanner(closer)
				scanner.Split(bufio.ScanLines)
				for scanner.Scan() {
					raw := scanner.Text()
					for _, data := range jsonextractor.ExtractStandardJSON(raw) {
						results := gjson.Parse(data).Get("content").String()
						w.Write([]byte(results))
					}
				}
				w.Close()
			}),
		)
		if err != nil {
			tokenCached = ""
			log.Warnf("failed to chat(comate): %v", err)
		}
		_ = rsp
	}()
	return r, nil
}

func (c *Client) Chat(s string, function ...any) (string, error) {
	reader, err := c.question(s)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer

	if c.config != nil {
		if c.config.StreamHandler != nil {
			teeReader := io.TeeReader(reader, &buf)
			c.config.StreamHandler(teeReader)
			return buf.String(), nil
		}
	}
	io.Copy(&buf, reader)
	return buf.String(), nil
}

func (c *Client) ChatStream(s string) (io.Reader, error) {
	return c.question(s)
}

func (c *Client) ExtractData(data string, desc string, fields map[string]any) (map[string]any, error) {
	prompt := aispec.GenerateJSONPrompt(data+"\n"+desc, fields)
	result, err := c.Chat(prompt)
	if err != nil && result == "" {
		return nil, err
	}
	return aispec.ExtractFromResult(result, fields)
}

func (c *Client) LoadOption(opt ...aispec.AIConfigOption) {
	config := aispec.NewDefaultAIConfig(opt...)

	log.Debug("load option for comate ai")
	c.config = config

	if c.config.Model == "" {
		c.config.Model = "ernie-bot"
	}
}

func (c *Client) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	return nil, nil
}

func (c *Client) CheckValid() error {
	var tokenNow string
	var err error

	if c.config == nil {
		if tokenNow, err = getToken(""); err != nil {
			return err
		}
	} else {
		tokenNow, err = getToken(c.config.APIKey)
		if err != nil {
			return err
		}
	}
	if tokenNow == "" || len(tokenNow) < 24 {
		return utils.Error("invalid token: " + tokenNow)
	}
	return nil
}

var _ aispec.AIClient = &Client{}
