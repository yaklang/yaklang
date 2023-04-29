package larkrobot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-resty/resty/v2"
	"strconv"

	"time"
)

// Client feishu robot client
type Client struct {
	// Webhook robot webhook address
	Webhook string
	// Secret robot secret
	Secret string
	client *resty.Client
}

// NewClient create Client
func NewClient(webhook string) *Client {
	return &Client{
		client:  resty.New(),
		Webhook: webhook,
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
	resp, err := c.client.R().
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetResult(&Response{}).
		ForceContentType("application/json").
		SetBody(body).
		Post(url)
	if err != nil {
		return nil, err
	}
	result := resp.Result().(*Response)
	return result, err
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
	Code int64  `json:"code"`
	Msg  string `json:"msg"`
}

// IsSuccess is success
func (r *Response) IsSuccess() bool {
	return r.Code == 0
}
