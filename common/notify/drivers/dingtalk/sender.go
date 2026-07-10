package dingtalk

import (
	"encoding/json"
	"fmt"
	neturl "net/url"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/internal/httpclient"
)

// Client 是钉钉平台实现，提供钉钉 HTTP/WS 底层能力。
type Client struct {
	mu     sync.Mutex
	tokens *tokenManager
	cfg    *notify.SendConfig

	eventHandler notify.EventHandler
}

type platformMessage struct {
	TargetID      string
	ReceiveIDType string
	MsgType       notify.MsgType
	Content       string
	Card          *notify.Card
	NativeCard    *notify.NativeCard
	IsGroup       bool
}

// New 构造一个钉钉客户端。cfg 可为 nil（延迟到发送时校验）。
func New(opts ...notify.SendOption) *Client {
	cfg := notify.NewSendConfig(opts...)
	return &Client{
		cfg:    cfg,
		tokens: newTokenManager(cfg),
	}
}

// GetType 返回平台类型。
func (c *Client) GetType() notify.PlatformType { return notify.PlatformDingTalk }

// Capabilities 声明钉钉平台固有能力。钉钉支持 sessionWebhook 轻量回复（归为
// NativeReply）、自定义表情 reaction，但不支持原生交互卡片（CardActions=false，
// UpdateCard=false），IM Engine 会走 TextRunPresenter 文本降级。
func (c *Client) Capabilities() notify.PlatformCapabilities {
	return notify.PlatformCapabilities{
		NativeReply: true,
		Reactions:   true,
		SendCard:    false,
		UpdateCard:  false,
		CardActions: false,
	}
}

// Configure 注入/更新运行时凭证。
func (c *Client) Configure(cfg *notify.SendConfig) {
	if cfg == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = cfg
	c.tokens = newTokenManager(cfg)
}

// SetEventHandler 注入接收循环的运行态事件回调。
func (c *Client) SetEventHandler(handler notify.EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventHandler = handler
}

func (c *Client) emitEvent(ev notify.Event) {
	c.mu.Lock()
	handler := c.eventHandler
	c.mu.Unlock()
	if handler != nil {
		handler(ev)
	}
}

func (c *Client) base() string {
	if c.cfg.BaseURL != "" {
		return c.cfg.BaseURL
	}
	return dingtalkAPI
}

// Send 按消息目标（单聊/群聊）路由到对应 REST 端点。
//
// 钉钉开放平台发送端点：
//   - 单聊: POST /v1.0/robot/oToMessages/batchSend   (需 access_token, 发给指定 staffId/outerUserId)
//   - 群聊: POST /v1.0/robot/groupMessages/send      (需 access_token, 发给 conversationId)
//
// 自定义群机器人 webhook + 加签模式（robotSecret 非空时）走另一条简单路径。
func (c *Client) Send(msg *platformMessage, config *notify.SendConfig) (*notify.SendResult, error) {
	if msg == nil {
		return nil, fmt.Errorf("dingtalk: message is nil")
	}
	cfg := c.effectiveConfig(config)

	// 自定义群机器人 webhook（仅有 secret / 目标即 webhook 时）：留作后续扩展，
	// 当前首推开放平台 App 模式。
	if cfg.RobotSecret != "" {
		return c.sendByRobotWebhook(msg, cfg)
	}
	if msg.IsGroup {
		return c.sendGroup(msg, cfg)
	}
	return c.sendSingle(msg, cfg)
}

// ReplyMessage 使用钉钉 reply context 回复消息。
// 钉钉机器人入站回复的官方路径是 sessionWebhook：收到用户消息时回调会带一个临时 webhook，
// POST 到它即可回到当前会话，无需 access_token。开放平台的主动发送接口用于新消息发送，
// 不提供飞书那种原生引用回复语义；没有 sessionWebhook 时才按普通主动发送回退。
func (c *Client) ReplyMessage(messageID string, msg *platformMessage, config *notify.SendConfig) (*notify.SendResult, error) {
	if msg == nil {
		return nil, fmt.Errorf("dingtalk: message is nil")
	}
	var rc replyContext
	if json.Unmarshal([]byte(messageID), &rc) == nil {
		if rc.SessionWebhook != "" {
			return c.replyViaSessionWebhook(rc.SessionWebhook, msg)
		}
		cfg := c.effectiveConfig(config)
		if rc.IsGroup {
			if rc.ConversationID == "" {
				return nil, fmt.Errorf("dingtalk: group reply fallback requires conversationId")
			}
			msg.TargetID = rc.ConversationID
			msg.IsGroup = true
			return c.sendGroup(msg, cfg)
		}
		if rc.SenderStaffID != "" {
			msg.TargetID = rc.SenderStaffID
			msg.IsGroup = false
			return c.sendSingle(msg, cfg)
		}
		return nil, fmt.Errorf("dingtalk: direct reply fallback requires sessionWebhook or senderStaffId")
	}
	// 非钉钉 replyContext：按普通发送处理，保留显式传入 TargetID/IsGroup 的调用能力。
	cfg := c.effectiveConfig(config)
	if cfg.RobotSecret != "" {
		return c.sendByRobotWebhook(msg, cfg)
	}
	if msg.IsGroup {
		return c.sendGroup(msg, cfg)
	}
	return c.sendSingle(msg, cfg)
}

// replyViaSessionWebhook 直接 POST 到钉钉 sessionWebhook 回复消息，无需 access_token。
// 对齐 cc-connect Reply（dingtalk.go:819）：始终用 markdown msgtype（钉钉 markdown 兼容纯文本，
// 且能渲染 **加粗**/# 标题/列表等富文本，AI 回复通常含 markdown 语法）。
// 注意：sessionWebhook URL 带查询参数（如 ?session=xxx），httpclient.Do 内部调
// ReplaceAllHTTPPacketQueryParams(req, query) 会覆盖 URL 上的 query，所以必须把
// URL 上的 query 提取出来通过 query 参数传进去，否则 session 参数丢失导致 errcode=40035。
func (c *Client) replyViaSessionWebhook(webhookURL string, msg *platformMessage) (*notify.SendResult, error) {
	title := "reply"
	if msg.Card != nil && msg.Card.Title != "" {
		title = msg.Card.Title
	}
	payload := map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]string{"title": title, "text": msg.Content},
	}
	// 提取 URL 上的 query 参数，通过 httpclient.Do 的 query 参数传入，避免被覆盖丢失。
	urlObj, err := neturl.Parse(webhookURL)
	if err != nil {
		return nil, fmt.Errorf("dingtalk: parse sessionWebhook url: %w", err)
	}
	query := map[string]string{}
	for k, vs := range urlObj.Query() {
		if len(vs) > 0 {
			query[k] = vs[0]
		}
	}
	// 用去掉 query 的 base URL，query 单独传
	baseURL := webhookURL
	if urlObj.RawQuery != "" {
		baseURL = strings.TrimSuffix(webhookURL, "?"+urlObj.RawQuery)
	}
	result, err := httpclient.Do("POST", baseURL, jsonHeaders(), query, payload)
	if err != nil {
		return nil, fmt.Errorf("dingtalk: reply via sessionWebhook: %w", err)
	}
	// 钉钉 sessionWebhook 返回 errcode!=0 时（如缺少参数 session），需当作失败处理。
	var respErr struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	_ = json.Unmarshal(result.Body, &respErr)
	if respErr.ErrCode != 0 {
		return nil, fmt.Errorf("dingtalk: sessionWebhook reply failed errcode=%d: %s", respErr.ErrCode, respErr.ErrMsg)
	}
	return &notify.SendResult{Raw: result.Body, Platform: notify.PlatformDingTalk}, nil
}

// ---- 钉钉自定义表情（emotion/reply）----
//
// 钉钉机器人可以给用户消息添加自定义表情（如「开始任务」「思考中」），通过
// /v1.0/robot/emotion/reply 端点实现，需要 access_token。对齐 cc-connect
// platform/dingtalk/dingtalk.go:914 sendEmotion。

const (
	// emotionTypeCustomText 自定义文字表情（cc-connect 同值）。
	emotionTypeCustomText = 2
	// customTextEmotionID / customTextEmotionBackground 钉钉自定义文字表情的样式 ID（cc-connect 同值）。
	customTextEmotionID         = "2659900"
	customTextEmotionBackground = "im_bg_1"
)

type dingtalkTextEmotion struct {
	EmotionID    string `json:"emotionId"`
	EmotionName  string `json:"emotionName"`
	Text         string `json:"text"`
	BackgroundID string `json:"backgroundId"`
}

type dingtalkEmotionRequest struct {
	RobotCode          string              `json:"robotCode"`
	OpenMsgID          string              `json:"openMsgId"`
	OpenConversationID string              `json:"openConversationId"`
	EmotionType        int                 `json:"emotionType"`
	EmotionName        string              `json:"emotionName"`
	TextEmotion        dingtalkTextEmotion `json:"textEmotion"`
}

// AddReaction 给钉钉消息添加自定义文字表情。
// messageID 是 IM Engine 透传的 replyContext JSON（含 msgId/conversationId/robotCode）。
// emojiType 是表情文字（如 "开始任务"），钉钉会渲染为自定义文字表情气泡。
func (c *Client) AddReaction(messageID, emojiType string) error {
	if messageID == "" || emojiType == "" {
		return fmt.Errorf("dingtalk: message_id and emoji_type are required")
	}
	var rc replyContext
	if json.Unmarshal([]byte(messageID), &rc) == nil {
		return c.sendEmotion(rc, emojiType, false)
	}
	return fmt.Errorf("dingtalk: AddReaction: cannot parse replyContext from messageID")
}

// sendEmotion 调钉钉 emotion API 给消息加/撤回自定义表情。
// recall=false 时添加表情，recall=true 时撤回（用 /v1.0/robot/emotion/recall 端点）。
func (c *Client) sendEmotion(rc replyContext, emoji string, recall bool) error {
	if rc.MsgID == "" || rc.ConversationID == "" {
		return fmt.Errorf("dingtalk: emotion requires msgId and conversationId")
	}
	robotCode := rc.RobotCode
	if robotCode == "" {
		robotCode = c.cfg.AppID // 默认 robotCode = appKey
	}
	token, err := c.tokens.getToken()
	if err != nil {
		return fmt.Errorf("dingtalk: get token for emotion: %w", err)
	}
	path := "/v1.0/robot/emotion/reply"
	if recall {
		path = "/v1.0/robot/emotion/recall"
	}
	body := dingtalkEmotionRequest{
		RobotCode:          robotCode,
		OpenMsgID:          rc.MsgID,
		OpenConversationID: rc.ConversationID,
		EmotionType:        emotionTypeCustomText,
		EmotionName:        emoji,
		TextEmotion: dingtalkTextEmotion{
			EmotionID:    customTextEmotionID,
			EmotionName:  emoji,
			Text:         emoji,
			BackgroundID: customTextEmotionBackground,
		},
	}
	headers := jsonHeaders()
	headers["x-acs-dingtalk-access-token"] = token
	url := c.base() + path
	result, err := httpclient.Do("POST", url, headers, nil, body, buildHTTPOpts(c.cfg.Proxy, c.cfg.Timeout)...)
	if err != nil {
		return fmt.Errorf("dingtalk: emotion request: %w", err)
	}
	if result.StatusCode != 200 {
		return fmt.Errorf("dingtalk: emotion status %d: %s", result.StatusCode, string(result.Body))
	}
	var resp struct {
		Success *bool `json:"success"`
	}
	if json.Unmarshal(result.Body, &resp) == nil && resp.Success != nil && !*resp.Success {
		return fmt.Errorf("dingtalk: emotion returned success=false: %s", string(result.Body))
	}
	return nil
}
func (c *Client) effectiveConfig(config *notify.SendConfig) *notify.SendConfig {
	c.mu.Lock()
	base := c.cfg
	c.mu.Unlock()
	if config == nil {
		return base
	}
	merged := *base
	if config.AppID != "" {
		merged.AppID = config.AppID
	}
	if config.AppSecret != "" {
		merged.AppSecret = config.AppSecret
	}
	if config.RobotSecret != "" {
		merged.RobotSecret = config.RobotSecret
	}
	if config.Proxy != "" {
		merged.Proxy = config.Proxy
	}
	if config.BaseURL != "" {
		merged.BaseURL = config.BaseURL
	}
	if config.Timeout != 0 {
		merged.Timeout = config.Timeout
	}
	// 用合并后的配置重建 token manager，保证凭证切换生效（与飞书实现一致）。
	c.mu.Lock()
	c.cfg = &merged
	c.tokens = newTokenManager(&merged)
	c.mu.Unlock()
	return &merged
}

// oToMsgBody / groupMsgBody 是钉钉发送请求体。
type oToMsgBody struct {
	RobotCode string   `json:"robotCode"`
	UserIDs   []string `json:"userIds,omitempty"`
	MsgKey    string   `json:"msgKey"`
	MsgParam  string   `json:"msgParam"` // 必须是 JSON 字符串
}

type groupMsgBody struct {
	ChatBotID          string `json:"chatbotId,omitempty"`
	OpenConversationID string `json:"openConversationId"`
	MsgKey             string `json:"msgKey"`
	MsgParam           string `json:"msgParam"`
}

type sendResp struct {
	ProcessQueryKey string `json:"processQueryKey"`
	MessageID       string `json:"messageId"`
}

// buildMsgKeyParam 根据 msg.MsgType 构造钉钉的 msgKey 和 msgParam(JSON 字符串)。
func buildMsgKeyParam(msg *platformMessage) (msgKey, msgParam string, err error) {
	switch msg.MsgType {
	case notify.MsgText, "":
		p := map[string]string{"content": msg.Content}
		b, _ := json.Marshal(p)
		return "sampleText", string(b), nil
	case notify.MsgMarkdown:
		p := map[string]string{"title": "markdown", "text": msg.Content}
		if msg.Card != nil && msg.Card.Title != "" {
			p["title"] = msg.Card.Title
		}
		b, _ := json.Marshal(p)
		return "sampleMarkdown", string(b), nil
	case notify.MsgCard:
		title := ""
		if msg.Card != nil {
			title = msg.Card.Title
		}
		p := map[string]string{"title": title, "text": cardText(msg)}
		b, _ := json.Marshal(p)
		return "sampleMarkdown", string(b), nil
	default:
		return "", "", fmt.Errorf("dingtalk: unsupported msg type %q", msg.MsgType)
	}
}

func cardText(msg *platformMessage) string {
	if msg.Card != nil && msg.Card.Content != "" {
		return msg.Card.Content
	}
	return msg.Content
}

// sendSingle 发送单聊消息。
// 钉钉新版 API 要求 token 放在请求头 x-acs-dingtalk-access-token（不再支持 query ?access_token=）。
func (c *Client) sendSingle(msg *platformMessage, cfg *notify.SendConfig) (*notify.SendResult, error) {
	token, err := c.tokens.getToken()
	if err != nil {
		return nil, err
	}
	msgKey, msgParam, err := buildMsgKeyParam(msg)
	if err != nil {
		return nil, err
	}
	body := oToMsgBody{
		RobotCode: cfg.AppID, // 钉钉 robotCode 通常等于 appKey
		UserIDs:   []string{msg.TargetID},
		MsgKey:    msgKey,
		MsgParam:  msgParam,
	}
	headers := jsonHeaders()
	headers["x-acs-dingtalk-access-token"] = token
	url := c.base() + "/v1.0/robot/oToMessages/batchSend"
	result, err := httpclient.Do("POST", url, headers, nil, body, buildHTTPOpts(cfg.Proxy, cfg.Timeout)...)
	if err != nil {
		return nil, fmt.Errorf("dingtalk: send single message: %w", err)
	}
	if result.StatusCode != 200 {
		return nil, fmt.Errorf("dingtalk: send single message status %d: %s", result.StatusCode, string(result.Body))
	}
	var resp sendResp
	_ = json.Unmarshal(result.Body, &resp)
	log.Debugf("dingtalk: send single message done: %s", string(result.Body))
	return &notify.SendResult{
		MessageID: resp.MessageID,
		Raw:       result.Body,
		Platform:  notify.PlatformDingTalk,
	}, nil
}

// sendGroup 发送群聊消息。
// 钉钉新版 API 要求 token 放在请求头 x-acs-dingtalk-access-token（不再支持 query ?access_token=）。
func (c *Client) sendGroup(msg *platformMessage, cfg *notify.SendConfig) (*notify.SendResult, error) {
	token, err := c.tokens.getToken()
	if err != nil {
		return nil, err
	}
	msgKey, msgParam, err := buildMsgKeyParam(msg)
	if err != nil {
		return nil, err
	}
	body := groupMsgBody{
		OpenConversationID: msg.TargetID,
		MsgKey:             msgKey,
		MsgParam:           msgParam,
	}
	headers := jsonHeaders()
	headers["x-acs-dingtalk-access-token"] = token
	url := c.base() + "/v1.0/robot/groupMessages/send"
	result, err := httpclient.Do("POST", url, headers, nil, body, buildHTTPOpts(cfg.Proxy, cfg.Timeout)...)
	if err != nil {
		return nil, fmt.Errorf("dingtalk: send group message: %w", err)
	}
	if result.StatusCode != 200 {
		return nil, fmt.Errorf("dingtalk: send group message status %d: %s", result.StatusCode, string(result.Body))
	}
	var resp sendResp
	_ = json.Unmarshal(result.Body, &resp)
	return &notify.SendResult{
		MessageID: resp.MessageID,
		Raw:       result.Body,
		Platform:  notify.PlatformDingTalk,
	}, nil
}

// sendByRobotWebhook 走自定义群机器人 webhook + HMAC-SHA256 加签。
//
// 该模式不需要 access_token，仅需 webhook 地址 + 加签 secret。
// 这里把 TargetID 视作 webhook 的 access_token（oapi.dingtalk.com/robot/send?access_token=xxx）。
func (c *Client) sendByRobotWebhook(msg *platformMessage, cfg *notify.SendConfig) (*notify.SendResult, error) {
	if cfg.RobotSecret == "" {
		return nil, fmt.Errorf("dingtalk: robot webhook mode requires RobotSecret")
	}
	ts, sign, err := robotSign(cfg.RobotSecret)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("https://oapi.dingtalk.com/robot/send?access_token=%s&timestamp=%d&sign=%s", msg.TargetID, ts, sign)

	payload := map[string]any{}
	switch msg.MsgType {
	case notify.MsgText, "":
		payload["msgtype"] = "text"
		payload["text"] = map[string]string{"content": msg.Content}
	case notify.MsgMarkdown, notify.MsgCard:
		title := "markdown"
		if msg.Card != nil && msg.Card.Title != "" {
			title = msg.Card.Title
		}
		payload["msgtype"] = "markdown"
		payload["markdown"] = map[string]string{"title": title, "text": cardText(msg)}
	default:
		return nil, fmt.Errorf("dingtalk: robot webhook unsupported msg type %q", msg.MsgType)
	}

	result, err := httpclient.Do("POST", url, jsonHeaders(), nil, payload, buildHTTPOpts(cfg.Proxy, cfg.Timeout)...)
	if err != nil {
		return nil, fmt.Errorf("dingtalk: robot webhook send: %w", err)
	}
	if result.StatusCode != 200 {
		return nil, fmt.Errorf("dingtalk: robot webhook status %d: %s", result.StatusCode, string(result.Body))
	}
	return &notify.SendResult{Raw: result.Body, Platform: notify.PlatformDingTalk}, nil
}

func jsonHeaders() map[string]string {
	return map[string]string{"Content-Type": "application/json; charset=utf-8"}
}

// robotSign 计算自定义群机器人加签：sign = base64(HMAC-SHA256(key=secret, msg=timestamp+"\n"+secret))。
func robotSign(secret string) (timestamp int64, sign string, err error) {
	timestamp = time.Now().UnixMilli()
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	mac := hmacSHA256([]byte(secret), []byte(stringToSign))
	if mac == nil {
		return 0, "", fmt.Errorf("dingtalk: compute robot sign failed")
	}
	sign = base64Std(mac)
	return timestamp, sign, nil
}
