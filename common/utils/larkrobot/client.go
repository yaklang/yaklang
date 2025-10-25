package larkrobot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// Client feishu robot client
type Client struct {
	// Webhook robot webhook address
	Webhook string
	// Secret robot secret
	Secret string
	// session id
	SessionID string
}

// NewClient create Client
func NewClient(webhook string) *Client {
	return &Client{
		SessionID: uuid.New().String(),
		Webhook:   webhook,
	}
}

func (c *Client) SendMessage(message Message) (*Response, error) {
	return c.SendMessageByUrl(c.Webhook, message)
}
func (c *Client) SendMessageStr(message string) (*Response, error) {
	return c.SendMessageStrByUrl(c.Webhook, message)
}
func (c *Client) SendMessageByUrl(url string, message Message) (*Response, error) {
	if message == nil {
		return nil, errors.New("message missing")
	}
	body := message.ToMessageMap()
	if len(c.Secret) != 0 {
		timestamp := time.Now().Unix()
		sign, err := c.GenSign(c.Secret, timestamp)
		if err != nil {
			return nil, err
		}
		body["timestamp"] = strconv.FormatInt(timestamp, 10)
		body["sign"] = sign
	}
	return c.send(url, body)
}
func (c *Client) SendMessageStrByUrl(url, message string) (*Response, error) {
	var body map[string]interface{}
	err := json.Unmarshal([]byte(message), &body)
	if err != nil {
		return nil, err
	}
	if len(c.Secret) != 0 {
		timestamp := time.Now().Unix()
		sign, err := c.GenSign(c.Secret, timestamp)
		if err != nil {
			return nil, err
		}
		body["timestamp"] = strconv.FormatInt(timestamp, 10)
		body["sign"] = sign
	}
	return c.send(url, body)
}
func (c *Client) send(url string, body interface{}) (*Response, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, utils.Errorf("larkrobot send error: json.Marshal error: %v", err)
	}

	// 调试日志已移除

	req := lowhttp.BasicRequest()
	req = lowhttp.SetHTTPPacketUrl(req, url)
	req = lowhttp.ReplaceHTTPPacketMethod(req, "POST")
	req = lowhttp.ReplaceHTTPPacketHeader(req, "Accept", "application/json")
	req = lowhttp.ReplaceHTTPPacketHeader(req, "Content-Type", "application/json; charset=utf-8")
	req = lowhttp.ReplaceHTTPPacketHeader(req, "User-Agent", "yaklang-larkrobot/1.0")
	req = lowhttp.ReplaceHTTPPacketBody(req, bodyBytes, false)
	https := false
	if utils.IsHttpOrHttpsUrl(url) && strings.HasPrefix(strings.ToLower(url), "https://") {
		https = true
	}

	resp, err := lowhttp.HTTP(lowhttp.WithRequest(string(req)), lowhttp.WithHttps(https))
	if err != nil {
		return nil, utils.Errorf("larkrobot send error: http error: %v", err)
	}
	statusCode := lowhttp.GetStatusCodeFromResponse(resp.RawPacket)
	if statusCode != 200 {
		return nil, utils.Errorf("larkrobot send error: http status code: %v", statusCode)
	}

	respBodyBytes := lowhttp.GetHTTPPacketBody(resp.RawPacket)
	if strings.Contains(string(respBodyBytes), "err") {
		log.Infof("request body: %s", string(bodyBytes))
		log.Infof("larkrobot http request: \n%v", string(resp.RawRequest))
		log.Infof("larkrobot response: %s", string(respBodyBytes))
	}
	var result Response
	err = json.Unmarshal(respBodyBytes, &result)
	if err != nil {
		return nil, utils.Errorf("larkrobot send error: json.Unmarshal error: %v", err)
	}
	return &result, nil
}
func (c *Client) GenSign(secret string, timestamp int64) (string, error) {
	//timestamp + key 做sha256, 再进行base64 encode
	stringToSign := fmt.Sprintf("%v", timestamp) + "\n" + secret
	var data []byte
	h := hmac.New(sha256.New, []byte(stringToSign))
	_, err := h.Write(data)
	if err != nil {
		return "", err
	}
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return signature, nil
}

// Response response struct
type Response struct {
	Code          int64       `json:"code"`
	Msg           string      `json:"msg"`
	Data          interface{} `json:"data,omitempty"`
	StatusCode    int64       `json:"StatusCode,omitempty"`
	StatusMessage string      `json:"StatusMessage,omitempty"`
}

// IsSuccess is success
func (r *Response) IsSuccess() bool {
	// 检查新格式的 StatusCode (0 表示成功)
	if r.StatusCode == 0 {
		return true
	}
	// 检查旧格式的 Code (0 表示成功)
	return r.Code == 0
}
