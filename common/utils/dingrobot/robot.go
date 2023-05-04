package dingrobot

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

// Roboter is the interface implemented by Robot that can send multiple types of messages.
type Roboter interface {
	SendText(content string, atMobiles []string, isAtAll bool) error
	SendLink(title, text, messageURL, picURL string) error
	SendMarkdown(title, text string, atMobiles []string, isAtAll bool) error
	SendActionCard(title, text, singleTitle, singleURL, btnOrientation, hideAvatar string) error
	SetSecret(secret string)
	GetCurrentWebHook() string
}

// Robot represents a dingtalk custom robot that can send messages to groups.
type Robot struct {
	webHook string
	secret  string
}

// Fetcher Current Webhook
func (r *Robot) GetCurrentWebHook() string {
	return r.webHook
}

// NewRobot returns a roboter that can send messages.
func NewRobot(webHook string) Roboter {
	return &Robot{webHook: webHook}
}

// SetSecret set the secret to add additional signature when send request
func (r *Robot) SetSecret(secret string) {
	r.secret = secret
}

// SendText send a text type message.
func (r Robot) SendText(content string, atMobiles []string, isAtAll bool) error {
	return r.send(&textMessage{
		MsgType: msgTypeText,
		Text: textParams{
			Content: content,
		},
		At: atParams{
			AtMobiles: atMobiles,
			IsAtAll:   isAtAll,
		},
	})
}

// SendLink send a link type message.
func (r Robot) SendLink(title, text, messageURL, picURL string) error {
	return r.send(&linkMessage{
		MsgType: msgTypeLink,
		Link: linkParams{
			Title:      title,
			Text:       text,
			MessageURL: messageURL,
			PicURL:     picURL,
		},
	})
}

// SendMarkdown send a markdown type message.
func (r Robot) SendMarkdown(title, text string, atMobiles []string, isAtAll bool) error {
	return r.send(&markdownMessage{
		MsgType: msgTypeMarkdown,
		Markdown: markdownParams{
			Title: title,
			Text:  text,
		},
		At: atParams{
			AtMobiles: atMobiles,
			IsAtAll:   isAtAll,
		},
	})
}

// SendActionCard send a action card type message.
func (r Robot) SendActionCard(title, text, singleTitle, singleURL, btnOrientation, hideAvatar string) error {
	return r.send(&actionCardMessage{
		MsgType: msgTypeActionCard,
		ActionCard: actionCardParams{
			Title:          title,
			Text:           text,
			SingleTitle:    singleTitle,
			SingleURL:      singleURL,
			BtnOrientation: btnOrientation,
			HideAvatar:     hideAvatar,
		},
	})
}

type dingResponse struct {
	Errcode int    `json:"errcode"`
	Errmsg  string `json:"errmsg"`
}

func (r Robot) send(msg interface{}) error {
	m, err := json.Marshal(msg)
	if err != nil {
		return utils.Errorf("marshal ding request failed: %s", err)
	}

	webURL := r.webHook
	if len(r.secret) != 0 {
		webURL += genSignedURL(r.secret)
	}

	resp, err := http.Post(webURL, "application/json", bytes.NewReader(m))
	if err != nil {
		return utils.Errorf("post %v failed: %s", webURL, err)
	}
	defer resp.Body.Close()

	log.Infof("dingding robot send with code: %v", resp.StatusCode)

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return utils.Errorf("read ding body failed: %s", err)
	}

	var dr dingResponse
	err = json.Unmarshal(data, &dr)
	if err != nil {
		return utils.Errorf("unmarshal ding response failed: %s", err)
	}
	if dr.Errcode != 0 {
		return fmt.Errorf("dingrobot send failed: %v", dr.Errmsg)
	}

	return nil
}

func genSignedURL(secret string) string {
	timeStr := fmt.Sprintf("%d", time.Now().UnixNano()/1e6)
	sign := fmt.Sprintf("%s\n%s", timeStr, secret)
	signData := computeHmacSha256(sign, secret)
	encodeURL := url.QueryEscape(signData)
	return fmt.Sprintf("&timestamp=%s&sign=%s", timeStr, encodeURL)
}

func computeHmacSha256(message string, secret string) string {
	key := []byte(secret)
	h := hmac.New(sha256.New, key)
	h.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
