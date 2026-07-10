package feishu

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/internal/httpclient"
)

// Client 是飞书平台实现，提供飞书 HTTP/WS 底层能力。
type Client struct {
	mu     sync.Mutex
	tokens *tokenManager
	cfg    *notify.SendConfig

	eventHandler notify.EventHandler

	// 合包缓存：飞书 WS 大消息会拆成多包（HeaderSum>1），按 message_id 缓存分包，
	// 收齐所有 seq 后拼接。combineMu 保护 combineCache 的并发访问。
	combineMu    sync.Mutex
	combineCache map[string][][]byte

	// wsWriteMu 串行化 WS 写操作（ping + ACK），对齐官方 SDK 的 c.mu。
	// lowhttp.WebsocketClient 的 WriteBinary 无锁，ping ticker goroutine 与
	// allFrameHandler goroutine 并发写会导致帧交错损坏、TLS 连接被关闭。
	wsWriteMu sync.Mutex
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

// New 构造飞书客户端。
func New(opts ...notify.SendOption) *Client {
	cfg := notify.NewSendConfig(opts...)
	return &Client{
		cfg:          cfg,
		tokens:       newTokenManager(cfg),
		combineCache: map[string][][]byte{},
	}
}

// GetType 返回平台类型。
func (c *Client) GetType() notify.PlatformType { return notify.PlatformFeishu }

// Capabilities 声明飞书平台固有能力。飞书支持交互卡片（schema 2.0）、
// 卡片更新（patch message_id）、卡片按钮回调（长连接 card.action.trigger）、
// reaction 表情回执、reply 引用回复。
func (c *Client) Capabilities() notify.PlatformCapabilities {
	return notify.PlatformCapabilities{
		NativeReply: true,
		Reactions:   true,
		SendCard:    true,
		UpdateCard:  true,
		CardActions: true,
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
	return feishuOpen
}

func (c *Client) loadConfig() *notify.SendConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

// effectiveConfig 合并实例配置与本次调用配置。
func (c *Client) effectiveConfig(config *notify.SendConfig) *notify.SendConfig {
	base := c.loadConfig()
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
	if config.Proxy != "" {
		merged.Proxy = config.Proxy
	}
	if config.BaseURL != "" {
		merged.BaseURL = config.BaseURL
	}
	if config.Timeout != 0 {
		merged.Timeout = config.Timeout
	}
	// 用合并后的配置重建 token manager，保证凭证切换生效。
	c.mu.Lock()
	c.cfg = &merged
	c.tokens = newTokenManager(&merged)
	c.mu.Unlock()
	return &merged
}

// imSendResp 飞书 im/v1/messages 响应。
type imSendResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		MessageID string `json:"message_id"`
	} `json:"data"`
}

// Send 通过 im/v1/messages 发送消息。
//
// POST /open-apis/im/v1/messages?receive_id_type=<type>
// Authorization: Bearer <tenant_access_token>
// body: {"receive_id": "...", "msg_type": "text", "content": "<json string>"}
func (c *Client) Send(msg *platformMessage, config *notify.SendConfig) (*notify.SendResult, error) {
	if msg == nil {
		return nil, fmt.Errorf("feishu: message is nil")
	}
	cfg := c.effectiveConfig(config)
	token, err := c.tokens.getToken()
	if err != nil {
		return nil, err
	}

	receiveIDType := msg.ReceiveIDType
	if receiveIDType == "" {
		receiveIDType = "open_id" // 默认按 open_id，用户可改 chat_id/user_id/email 等
	}
	msgType, content, err := buildFeishuContent(msg)
	if err != nil {
		return nil, err
	}

	body := map[string]string{
		"receive_id": msg.TargetID,
		"msg_type":   msgType,
		"content":    content, // 飞书要求 content 是 JSON 字符串
	}
	// receive_id_type 通过 query 参数传（不能用 URL 拼接，httpclient.Do 会用
	// ReplaceAllHTTPPacketQueryParams 覆盖掉 URL 里的 query）。
	url := c.base() + "/open-apis/im/v1/messages"
	query := map[string]string{"receive_id_type": receiveIDType}
	headers := jsonHeaders()
	headers["Authorization"] = "Bearer " + token
	result, err := httpclient.Do("POST", url, headers, query, body, buildHTTPOpts(cfg.Proxy, cfg.Timeout)...)
	if err != nil {
		return nil, fmt.Errorf("feishu: send message: %w", err)
	}
	if result.StatusCode != 200 {
		return nil, fmt.Errorf("feishu: send status %d: %s", result.StatusCode, string(result.Body))
	}
	var resp imSendResp
	if err := json.Unmarshal(result.Body, &resp); err != nil {
		return nil, fmt.Errorf("feishu: decode send resp: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("feishu: send failed code=%d msg=%s", resp.Code, resp.Msg)
	}
	log.Debugf("feishu: send message done: %s", string(result.Body))
	return &notify.SendResult{
		MessageID: resp.Data.MessageID,
		Raw:       result.Body,
		Platform:  notify.PlatformFeishu,
	}, nil
}

// buildFeishuContent 根据消息类型构造飞书的 msg_type + content(JSON 字符串)。
func buildFeishuContent(msg *platformMessage) (msgType, content string, err error) {
	if msg.NativeCard != nil {
		if msg.NativeCard.Platform != "" && msg.NativeCard.Platform != notify.PlatformFeishu {
			return "", "", fmt.Errorf("feishu: native card platform %q is not feishu", msg.NativeCard.Platform)
		}
		if msg.NativeCard.Schema != "" && msg.NativeCard.Schema != "feishu.card.v2" {
			return "", "", fmt.Errorf("feishu: unsupported native card schema %q", msg.NativeCard.Schema)
		}
		if len(msg.NativeCard.Body) == 0 {
			return "", "", fmt.Errorf("feishu: native card body is empty")
		}
		if !json.Valid(msg.NativeCard.Body) {
			return "", "", fmt.Errorf("feishu: native card body must be valid json")
		}
		return "interactive", string(msg.NativeCard.Body), nil
	}
	switch msg.MsgType {
	case notify.MsgText, "":
		// 如果内容含 markdown 语法，用 Card JSON 2.0 卡片消息（markdown element 原生支持 # 标题等）
		if containsMarkdown(msg.Content) {
			return buildFeishuMarkdownCard(msg.Content)
		}
		b, _ := json.Marshal(map[string]string{"text": msg.Content})
		return "text", string(b), nil
	case notify.MsgMarkdown:
		// markdown 走 Card JSON 2.0（飞书卡片 markdown element 原生支持 # 标题等，仅 2.0 可用）
		return buildFeishuMarkdownCard(msg.Content)
	case notify.MsgCard:
		// Card JSON 2.0：header + body.elements。默认用 markdown element；
		// 高级调用方可传 Card.Elements 原样渲染 markdown/column_set 等 V2 支持的元素。
		var elements []map[string]any
		if msg.Card != nil {
			elements = append(elements, msg.Card.Elements...)
		}
		if len(elements) == 0 {
			elements = []map[string]any{{"tag": "markdown", "content": cardMarkdown(msg)}}
		}
		if msg.Card != nil && len(msg.Card.Buttons) > 0 {
			// Card JSON 2.0 推荐把 button 放在 column_set -> column 中，
			// 按钮本身用 behaviors:[{type:"callback", value:...}] 回传 action.value。
			if btns := buildFeishuCardButtons(msg.Card.Buttons); btns != nil {
				elements = append(elements, btns)
			}
		}
		card := map[string]any{
			"schema": "2.0",
			"body":   map[string]any{"elements": elements},
		}
		if msg.Card != nil && len(msg.Card.Config) > 0 {
			card["config"] = msg.Card.Config
		}
		if msg.Card != nil && msg.Card.Title != "" {
			card["header"] = map[string]any{
				"title": map[string]string{"tag": "plain_text", "content": msg.Card.Title},
			}
		}
		b, _ := json.Marshal(card)
		return "interactive", string(b), nil
	default:
		return "", "", fmt.Errorf("feishu: unsupported msg type %q", msg.MsgType)
	}
}

// containsMarkdown 检测文本是否包含 markdown 语法（**加粗** / `代码` / # 标题 / - 列表 / [链接](url)）。
func containsMarkdown(s string) bool {
	if strings.Contains(s, "**") || strings.Contains(s, "`") {
		return true
	}
	// 标题（行首 # ）
	if strings.Contains(s, "\n# ") || strings.HasPrefix(s, "# ") {
		return true
	}
	// 列表（行首 - 或 * ）
	if strings.Contains(s, "\n- ") || strings.Contains(s, "\n* ") {
		return true
	}
	// 链接 [text](url)
	if strings.Contains(s, "](") {
		return true
	}
	return false
}

// buildFeishuMarkdownCard 构造飞书 Card JSON 2.0 卡片消息，用 markdown element 渲染富文本。
// 飞书卡片 markdown element 在 2.0 schema 下原生支持 **加粗** / `代码` / [链接](url) /
// # 标题 / 列表 / 引用块 / 代码块 / 表格 等完整 markdown 语法（1.0 仅支持部分），
// 且在 reply API 中也被接受（不像 post+lark_md 会报 wrong tag）。
// 结构参考官方 SDK oapi-sdk-go/service/cardkit/v1/model.go：
//
//	{"schema":"2.0","body":{"elements":[{"tag":"markdown","content":"..."}]}}
//
// 返回 (msgType="interactive", content=card JSON 字符串, nil)。
func buildFeishuMarkdownCard(mdContent string) (string, string, error) {
	card := map[string]any{
		"schema": "2.0",
		"body": map[string]any{
			"elements": []map[string]string{{"tag": "markdown", "content": mdContent}},
		},
	}
	b, err := json.Marshal(card)
	if err != nil {
		return "", "", fmt.Errorf("feishu: marshal markdown card: %w", err)
	}
	return "interactive", string(b), nil
}

func cardMarkdown(msg *platformMessage) string {
	if msg.Card != nil && msg.Card.Content != "" {
		return msg.Card.Content
	}
	return msg.Content
}

// buildFeishuCardButton 构造飞书 schema 2.0 的 button 元素。
// 点击后飞书经长连接/HTTP 回传 behaviors[0].value 原样到 card.action.trigger 事件，
// 由 receiver 的 dispatchCardAction 解析为 InboundMessage.ActionValue。
//
// 结构：{"tag":"button","text":{"tag":"plain_text","content":Text},
//
//	"type":"<Style>","behaviors":[{"type":"callback","value":<Value>}]}
func buildFeishuCardButton(btn notify.CardButton) map[string]any {
	button := map[string]any{
		"tag": "button",
		"text": map[string]string{
			"tag":     "plain_text",
			"content": btn.Text,
		},
		"behaviors": []map[string]any{
			{"type": "callback", "value": btn.Value},
		},
	}
	if btn.Style != "" {
		button["type"] = btn.Style // primary / danger / default
	}
	return button
}

func buildFeishuCardButtons(buttons []notify.CardButton) map[string]any {
	if len(buttons) == 0 {
		return nil
	}
	columns := make([]map[string]any, 0, len(buttons))
	for _, btn := range buttons {
		columns = append(columns, map[string]any{
			"tag":            "column",
			"width":          "auto",
			"vertical_align": "top",
			"elements":       []map[string]any{buildFeishuCardButton(btn)},
		})
	}
	return map[string]any{
		"tag":       "column_set",
		"flex_mode": "none",
		"columns":   columns,
	}
}

// PatchCard 更新一张已发送的交互卡片内容（飞书 patch message API）。
//
// PATCH /open-apis/im/v1/messages/{message_id}
// body: {"content": "<新的 card JSON 字符串>"}  （content 为整张 schema 2.0 卡片 JSON）
// 不需要 msg_type / receive_id；content 由 buildFeishuContent 产出（interactive 类型）。
// 用于 managed run card 在 agent 运行期间更新状态/进度/结果。
//
// 注意：本方法不做节流——频控（飞书约 50 次/秒、1000 次/分钟，工程实践 ~500ms 一次）
// 由上层 RunPresenter 负责，避免底层方法隐藏时延。
func (c *Client) PatchCard(messageID string, msg *platformMessage, config *notify.SendConfig) (*notify.SendResult, error) {
	if messageID == "" {
		return nil, fmt.Errorf("feishu: message_id is required for patch card")
	}
	if msg == nil {
		return nil, fmt.Errorf("feishu: message is nil")
	}
	cfg := c.effectiveConfig(config)
	token, err := c.tokens.getToken()
	if err != nil {
		return nil, err
	}

	// patch 卡片强制走 interactive：若调用方给了 MsgText/MsgMarkdown，
	// 也升级成卡片（patch 只支持 interactive 卡片，不支持纯文本）。
	effMsg := *msg
	if effMsg.MsgType != notify.MsgCard {
		effMsg.MsgType = notify.MsgCard
		if effMsg.Card == nil {
			effMsg.Card = &notify.Card{Content: msg.Content}
		}
	}
	_, content, err := buildFeishuContent(&effMsg)
	if err != nil {
		return nil, err
	}

	body := map[string]string{"content": content}
	url := c.base() + "/open-apis/im/v1/messages/" + messageID
	headers := jsonHeaders()
	headers["Authorization"] = "Bearer " + token
	result, err := httpclient.Do("PATCH", url, headers, nil, body, buildHTTPOpts(cfg.Proxy, cfg.Timeout)...)
	if err != nil {
		return nil, fmt.Errorf("feishu: patch card: %w", err)
	}
	if result.StatusCode != 200 {
		return nil, fmt.Errorf("feishu: patch card status %d: %s", result.StatusCode, string(result.Body))
	}
	var resp imSendResp
	if err := json.Unmarshal(result.Body, &resp); err != nil {
		return nil, fmt.Errorf("feishu: decode patch resp: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("feishu: patch card failed code=%d msg=%s", resp.Code, resp.Msg)
	}
	log.Debugf("feishu: patch card done: %s", string(result.Body))
	return &notify.SendResult{
		MessageID: messageID, // patch 不产生新 message_id
		Raw:       result.Body,
		Platform:  notify.PlatformFeishu,
	}, nil
}

// AddReaction 给指定消息添加一个表情回应（reaction）。
// messageID 是飞书消息 ID（om_xxx），emojiType 是飞书 emoji 类型
// （如 "OK"=👍, "WHITE_CHECK_MARK"=✅, "DONE"=✔️）。
// 参考飞书 API：POST /open-apis/im/v1/messages/{message_id}/reactions
func (c *Client) AddReaction(messageID, emojiType string) error {
	if messageID == "" || emojiType == "" {
		return fmt.Errorf("feishu: message_id and emoji_type are required")
	}
	cfg := c.loadConfig()
	token, err := c.tokens.getToken()
	if err != nil {
		return err
	}
	url := c.base() + "/open-apis/im/v1/messages/" + messageID + "/reactions"
	// 飞书 reaction 请求体是嵌套结构：{"reaction_type":{"emoji_type":"XXX"}}
	// （参考 lark SDK model.go:4880 ReactionType *Emoji `json:"reaction_type"`）
	body := map[string]map[string]string{
		"reaction_type": {"emoji_type": emojiType},
	}
	headers := jsonHeaders()
	headers["Authorization"] = "Bearer " + token
	result, err := httpclient.Do("POST", url, headers, nil, body, buildHTTPOpts(cfg.Proxy, cfg.Timeout)...)
	if err != nil {
		return fmt.Errorf("feishu: add reaction: %w", err)
	}
	if result.StatusCode != 200 {
		return fmt.Errorf("feishu: add reaction status %d: %s", result.StatusCode, string(result.Body))
	}
	return nil
}

// ReplyMessage 引用回复指定消息。飞书 reply API：POST /open-apis/im/v1/messages/{message_id}/reply
// 与 Send 不同，reply 不需要 receive_id/receive_id_type，自动知道目标。
// 请求体只需 msg_type + content（参考 lark SDK ReplyMessageReqBody）。
func (c *Client) ReplyMessage(messageID string, msg *platformMessage, config *notify.SendConfig) (*notify.SendResult, error) {
	if messageID == "" {
		return nil, fmt.Errorf("feishu: message_id is required for reply")
	}
	if msg == nil {
		return nil, fmt.Errorf("feishu: message is nil")
	}
	cfg := c.effectiveConfig(config)
	token, err := c.tokens.getToken()
	if err != nil {
		return nil, err
	}

	msgType, content, err := buildFeishuContent(msg)
	if err != nil {
		return nil, err
	}

	body := map[string]string{
		"msg_type": msgType,
		"content":  content,
	}
	url := c.base() + "/open-apis/im/v1/messages/" + messageID + "/reply"
	headers := jsonHeaders()
	headers["Authorization"] = "Bearer " + token
	result, err := httpclient.Do("POST", url, headers, nil, body, buildHTTPOpts(cfg.Proxy, cfg.Timeout)...)
	if err != nil {
		return nil, fmt.Errorf("feishu: reply message: %w", err)
	}
	if result.StatusCode != 200 {
		return nil, fmt.Errorf("feishu: reply status %d: %s", result.StatusCode, string(result.Body))
	}
	var resp imSendResp
	if err := json.Unmarshal(result.Body, &resp); err != nil {
		return nil, fmt.Errorf("feishu: decode reply resp: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("feishu: reply failed code=%d msg=%s", resp.Code, resp.Msg)
	}
	return &notify.SendResult{
		MessageID: resp.Data.MessageID,
		Raw:       result.Body,
		Platform:  notify.PlatformFeishu,
	}, nil
}
