// Package utils
// @Author bcy2007  2023/9/21 13:52
package utils

import (
	"bytes"
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"net/http"
	"strings"
)

type Message struct {
	Message     interface{} `json:"content"`
	MessageType string      `json:"type"`
}

type Result struct {
	Code int               `json:"code"`
	Data map[string]string `json:"data"`
	Msg  string            `json:"msg"`
}

type HttpMessageSend struct {
	IPAddress string
	client    *http.Client
}

func NewMessageSender(address string) *HttpMessageSend {
	if !strings.HasPrefix(address, "http://") {
		address = "http://" + address
	}
	sender := &HttpMessageSend{
		IPAddress: address,
	}
	sender.init()
	return sender
}

func (sender *HttpMessageSend) init() {
	sender.client = http.DefaultClient
}

func (sender *HttpMessageSend) PretendSendMessages(messages interface{}, messageType string) error {
	message := &Message{Message: messages, MessageType: messageType}
	data, err := json.Marshal(message)
	if err != nil {
		return utils.Errorf("marshal message error: %v", err)
	}
	log.Info(string(data))
	return nil
}

func (sender *HttpMessageSend) SendMessages(messages interface{}, messageType string) error {
	messageBytes, err := json.Marshal(messages)
	if err != nil {
		return utils.Errorf("marshal message error: %v", err)
	}
	message := &Message{Message: string(messageBytes), MessageType: messageType}
	data, err := json.Marshal(message)
	if err != nil {
		return utils.Errorf("marshal message error: %v", err)
	}
	req, err := http.NewRequest("POST", sender.IPAddress+"/api/smart/bas/receivresult", bytes.NewReader(data))
	if err != nil {
		return utils.Errorf("create http post request error: %v", err)
	}
	req.Header.Set("content-type", "application/json")
	res, err := sender.client.Do(req)
	if err != nil {
		return utils.Errorf("client send http post request error: %v", err)
	}
	bodyBytes, err := io.ReadAll(res.Body)
	var result Result
	if err = json.Unmarshal(bodyBytes, &result); err != nil {
		return utils.Errorf("unmarshal post result error: %v", err)
	}
	if result.Code == 200 {
		return nil
	} else {
		return utils.Errorf("result error: %v", result)
	}
}
