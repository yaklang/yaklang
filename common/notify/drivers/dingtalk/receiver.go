package dingtalk

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/internal/httpclient"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// Stream 协议常量（对齐 dingtalk-stream-sdk-go 官方 SDK）。
// 官方 SDK payload/utils.go:27 BotMessageCallbackTopic = "/v1.0/im/bot/messages/get"
// 官方 SDK utils/utils.go:16 SubscriptionTypeKCallback = "CALLBACK"
const (
	// streamSubTypeCallback 是 gateway/connections/open 请求里 subscriptions 的 type，
	// 表示订阅「回调」类消息（机器人收消息属于此类）。
	streamSubTypeCallback = "CALLBACK"
	// topicBotMessage 是机器人消息统一回调 topic，对齐官方 BotMessageCallbackTopic。
	topicBotMessage = "/v1.0/im/bot/messages/get"
)

// gatewayOpenResp 是 POST gateway/connections/open 的响应。
// 对齐官方 SDK ConnectionEndpointResponse：endpoint/ticket 在顶层，不嵌套在 data 里。
type gatewayOpenResp struct {
	Code     string `json:"code"` // 钉钉可能返回 code（字符串）或没有该字段
	Endpoint string `json:"endpoint"`
	Ticket   string `json:"ticket"`
	Message  string `json:"message"`
}

// subscriptions 请求体里的订阅项。
type subscription struct {
	Type   string            `json:"type"`
	Topic  string            `json:"topic"`
	Extras map[string]string `json:"extras,omitempty"`
}

// gatewayReq 是 gateway/connections/open 请求体。
type gatewayReq struct {
	ClientID      string         `json:"clientId"`
	ClientSecret  string         `json:"clientSecret"`
	Subscriptions []subscription `json:"subscriptions"`
	UserAgent     string         `json:"userAgent,omitempty"`
}

// dataFrame 是 Stream 收到的文本帧 JSON（DataFrame）。
type dataFrame struct {
	Headers struct {
		ContentType   string `json:"contentType"`
		Topic         string `json:"topic"`
		MessageID     string `json:"messageId"`
		Time          string `json:"time"`
		EventRoamTime string `json:"eventRoamTime"`
		EventOrigin   string `json:"origin"`
		BizCID        string `json:"bizCuId"`
	} `json:"headers"`
	Data      string `json:"data"` // 业务负载 JSON 字符串
	MessageID string `json:"messageId"`
	Type      string `json:"type"`
	Topic     string `json:"topic"`
	Time      int64  `json:"time"`
}

// frameResponse 是回 ACK 给服务端的 DataFrameResponse JSON。
type frameResponse struct {
	Code    int `json:"code"`
	Headers struct {
		ContentType string `json:"contentType"`
		MessageID   string `json:"messageId"`
	} `json:"headers"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// botPayload 解析 dataFrame.Data 里的业务负载（bot 消息）。
// sessionWebhook 是钉钉回调带的临时回复 webhook（2h 有效），POST 到它即可回复，无需 access_token。
// senderStaffId 是主动单聊发送需要的用户 ID；senderId 在不同钉钉版本里可能是字符串
// （旧）或对象{staffId,unionId}（新），用 json.RawMessage 兼容兜底。
type botPayload struct {
	ConversationID string `json:"conversationId"`
	AtUsers        []struct {
		DingTalkID string `json:"dingtalkId"`
		StaffID    string `json:"staffId"`
	} `json:"atUsers"`
	ChatbotUserID string          `json:"chatbotUserId"`
	SenderStaffID string          `json:"senderStaffId"`
	SenderID      json.RawMessage `json:"senderId"`
	SenderNick    string          `json:"senderNick"`
	Admin         bool            `json:"isAdmin"`
	Text          struct {
		Content string `json:"content"`
	} `json:"text"`
	Content              json.RawMessage `json:"content"`
	MsgID                string          `json:"msgId"`
	RobotCode            string          `json:"robotCode"`
	SessionWebhook       string          `json:"sessionWebhook"`
	SessionWebhookExpire int64           `json:"sessionWebhookExpiredTime"`
	CreateAt             any             `json:"createAt"`
	ConversationType     string          `json:"conversationType"` // "1"=单聊 "2"=群
	IsInAtList           bool            `json:"isInAtList"`
	MsgType              string          `json:"msgtype"`
}

// Start 启动 Stream 接收：获取 gateway 地址 → 建 ws → 读循环 + ping/pong + ACK + 断线重连。
//
// 阻塞直到 ctx 取消或不可恢复错误。每收到一条 bot 消息回调 handler。
func (c *Client) Start(ctx context.Context, handler func(*notify.InboundMessage)) error {
	cfg := c.loadConfig()
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return fmt.Errorf("dingtalk: appKey/appSecret required for stream")
	}
	// 重连循环。
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := c.runStreamOnce(ctx, cfg, handler)
		if err == nil {
			err = fmt.Errorf("connection closed; reconnecting")
		}
		c.emitEvent(notify.Event{
			Type:     notify.EventError,
			Platform: notify.PlatformDingTalk,
			Err:      err,
		})
		log.Warnf("dingtalk: stream disconnected: %v", err)
		// 重连前等待，尊重 ctx。
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// loadConfig 读取当前配置快照。
func (c *Client) loadConfig() *notify.SendConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	snap := c.cfg
	return snap
}

// runStreamOnce 建立一次完整的 ws 会话，直到断开返回。
func (c *Client) runStreamOnce(ctx context.Context, cfg *notify.SendConfig, handler func(*notify.InboundMessage)) error {
	endpoint, ticket, err := c.openGateway(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open gateway: %w", err)
	}
	wsURL := endpoint
	if !strings.Contains(wsURL, "ticket=") {
		sep := "?"
		if strings.Contains(wsURL, "?") {
			sep = "&"
		}
		wsURL = wsURL + sep + "ticket=" + ticket
	}
	return c.runWS(ctx, wsURL, cfg, handler)
}

// openGateway POST gateway/connections/open 拿 endpoint+ticket。
func (c *Client) openGateway(ctx context.Context, cfg *notify.SendConfig) (endpoint, ticket string, err error) {
	base := c.base()
	req := gatewayReq{
		ClientID:     cfg.AppID,
		ClientSecret: cfg.AppSecret,
		UserAgent:    "yaklang-notify/1.0",
		Subscriptions: []subscription{
			{Type: streamSubTypeCallback, Topic: topicBotMessage},
		},
	}
	url := base + "/v1.0/gateway/connections/open"
	result, rerr := httpclient.Do("POST", url, jsonHeaders(), nil, req, buildHTTPOpts(cfg.Proxy, cfg.Timeout)...)
	if rerr != nil {
		return "", "", rerr
	}
	if result.StatusCode != 200 {
		return "", "", fmt.Errorf("gateway open status %d: %s", result.StatusCode, string(result.Body))
	}
	var resp gatewayOpenResp
	if err := json.Unmarshal(result.Body, &resp); err != nil {
		return "", "", fmt.Errorf("decode gateway resp: %w", err)
	}
	if resp.Endpoint == "" {
		return "", "", fmt.Errorf("empty gateway endpoint: %s", string(result.Body))
	}
	return resp.Endpoint, resp.Ticket, nil
}

// runWS 连接 ws 并处理帧。
func (c *Client) runWS(ctx context.Context, wsURL string, cfg *notify.SendConfig, handler func(*notify.InboundMessage)) error {
	// lowhttp 的 ws 客户端需要一个原始 HTTP 升级报文。这里把 ws/wss URL 转换。
	packet := buildWSUpgradePacket(wsURL)
	isTLS := strings.HasPrefix(wsURL, "wss://")

	wsOpts := []lowhttp.WebsocketClientOpt{
		lowhttp.WithWebsocketWithContext(ctx),
		lowhttp.WithWebsocketTLS(isTLS),
		lowhttp.WithWebsocketTotalTimeout(0),
	}
	if cfg.Proxy != "" {
		wsOpts = append(wsOpts, lowhttp.WithWebsocketProxy(cfg.Proxy))
	}
	// 心跳：ws 连接内部由 lowhttp 处理 ping/pong 控制帧。业务心跳在收到消息回调里做。
	lastMsg := time.Now()
	wsOpts = append(wsOpts, lowhttp.WithWebsocketFromServerHandlerEx(func(client *lowhttp.WebsocketClient, data []byte, frames []*lowhttp.Frame) {
		lastMsg = time.Now()
		c.handleStreamMessage(data, client, handler)
	}))

	client, err := lowhttp.NewWebsocketClient(packet, wsOpts...)
	if err != nil {
		return fmt.Errorf("ws dial: %w", err)
	}
	client.Start()
	c.emitEvent(notify.Event{
		Type:     notify.EventConnected,
		Platform: notify.PlatformDingTalk,
	})
	log.Infof("dingtalk: stream ws connected to %s", wsURL)

	// 心跳看护：长时间无消息主动发 ping（lowhttp 没有 idle ping，业务层补）。
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	_ = lastMsg // 仅供未来扩展使用
	<-client.Context.Done()
	return client.Close()
}

// handleStreamMessage 解析一条 Stream 文本帧，回 ACK，并把 bot 消息回调出去。
func (c *Client) handleStreamMessage(data []byte, client *lowhttp.WebsocketClient, handler func(*notify.InboundMessage)) {
	var df dataFrame
	if err := json.Unmarshal(data, &df); err != nil {
		log.Debugf("dingtalk: drop non-json frame: %v (raw=%s)", err, string(data))
		return
	}
	log.Debugf("dingtalk: recv frame type=%q topic=%q headers=%+v", df.Type, df.Topic, df.Headers)
	// system ping 帧（type 为 ping 或 message==pong）→ 回 pong，无需业务处理。
	if df.Type == "ping" || df.Headers.Topic == "ping" {
		c.writeACK(client, df.MessageID, nil)
		return
	}
	// disconnect 指令 → 关闭让上层重连。
	if df.Type == "disconnect" || df.Headers.Topic == "disconnect" {
		log.Infof("dingtalk: stream server requested disconnect, will reconnect")
		_ = client.Close()
		return
	}
	// 回 ACK。
	c.writeACK(client, df.MessageID, nil)

	// 只处理 bot 消息。
	if df.Headers.Topic != topicBotMessage && df.Topic != topicBotMessage {
		return
	}
	if dingtalkRawPayloadLogEnabled() && df.Data != "" {
		raw := df.Data
		if len(raw) > 8192 {
			raw = raw[:8192] + "...(truncated)"
		}
		log.Infof("dingtalk: raw bot payload: %s", raw)
	}
	if handler == nil {
		return
	}
	var bot botPayload
	if df.Data != "" {
		if err := json.Unmarshal([]byte(df.Data), &bot); err != nil {
			log.Warnf("dingtalk: decode bot payload: %v", err)
			return
		}
	}
	text, attachments := extractDingTalkContent(bot)
	mentionBot := bot.ConversationType == "2" && bot.IsInAtList
	if bot.ConversationType == "2" && !mentionBot {
		for _, at := range bot.AtUsers {
			if at.StaffID != "" && at.StaffID == bot.ChatbotUserID {
				mentionBot = true
				break
			}
			if at.DingTalkID != "" && at.DingTalkID == bot.ChatbotUserID {
				mentionBot = true
				break
			}
		}
	}
	if bot.ConversationType == "2" && !mentionBot && strings.HasPrefix(text, "@") {
		mentionBot = true
	}
	senderStaffID := strings.TrimSpace(bot.SenderStaffID)
	// senderId 在不同钉钉版本里可能是字符串（旧）或对象{staffId,unionId}（新）。
	senderID := extractSenderID(bot.SenderID)
	if senderID == "" {
		senderID = senderStaffID
	}
	if senderStaffID == "" {
		senderStaffID = senderID
	}
	inbound := &notify.InboundMessage{
		Platform:   notify.PlatformDingTalk,
		ChatID:     bot.ConversationID,
		SenderID:   senderID,
		SenderName: bot.SenderNick,
		Text:       text,
		EventTime:  parseDingTalkEventTime(bot.CreateAt, df.Headers.EventRoamTime, df.Headers.Time, df.Time),
		Raw:        []byte(df.Data),
		// ReplyContext 优先用 sessionWebhook（钉钉临时回复 webhook，2h 有效，POST 即回复，无需 access_token）。
		// 对齐 cc-connect replyContext.sessionWebhook 与官方 SDK chatbot_replier。
		// 回退主动发送时，群聊用 conversationId，单聊用 senderStaffId（需 access_token）。
		ReplyContext: replyContext{
			SessionWebhook: bot.SessionWebhook,
			ConversationID: bot.ConversationID,
			SenderStaffID:  senderStaffID,
			MsgID:          bot.MsgID,
			RobotCode:      bot.RobotCode,
			IsGroup:        bot.ConversationType == "2",
		},
		ChatType:    normalizeDingTalkChatType(bot.ConversationType),
		MentionBot:  mentionBot,
		Attachments: attachments,
	}
	handler(inbound)
}

func extractDingTalkContent(bot botPayload) (string, []notify.IMAttachment) {
	text := strings.TrimSpace(bot.Text.Content) // 钉钉 @robot 时前缀常带空白，只裁剪边界，保留命令参数空格。
	if len(bot.Content) == 0 {
		return text, nil
	}
	var contentMap map[string]any
	_ = json.Unmarshal(bot.Content, &contentMap)
	var attachments []notify.IMAttachment
	if bot.MsgType == "picture" || bot.MsgType == "image" {
		downloadCode, _ := contentMap["downloadCode"].(string)
		attachments = appendDingTalkImageAttachment(attachments, downloadCode, "", bot.MsgID)
	}
	var content struct {
		RichText []map[string]any `json:"richText"`
	}
	if err := json.Unmarshal(bot.Content, &content); err != nil || len(content.RichText) == 0 {
		return text, attachments
	}
	var parts []string
	for _, item := range content.RichText {
		if s, ok := item["text"].(string); ok {
			parts = append(parts, s)
			continue
		}
		typ, _ := item["type"].(string)
		downloadCode, _ := item["downloadCode"].(string)
		if typ == "picture" {
			pictureCode, _ := item["pictureDownloadCode"].(string)
			attachments = appendDingTalkImageAttachment(attachments, downloadCode, pictureCode, bot.MsgID)
		}
	}
	if len(parts) > 0 {
		text = strings.TrimSpace(strings.Join(parts, ""))
	}
	return text, attachments
}

func appendDingTalkImageAttachment(attachments []notify.IMAttachment, downloadCode, fileName, messageID string) []notify.IMAttachment {
	downloadCode = strings.TrimSpace(downloadCode)
	if downloadCode == "" {
		return attachments
	}
	return append(attachments, notify.IMAttachment{
		Type:      notify.MsgImage,
		FileKey:   downloadCode,
		FileName:  strings.TrimSpace(fileName),
		MessageID: messageID,
	})
}

func parseDingTalkEventTime(values ...any) time.Time {
	for _, value := range values {
		switch v := value.(type) {
		case string:
			if ts := parseDingTalkEventTimeValue(v); !ts.IsZero() {
				return ts
			}
		case int64:
			if ts := parseDingTalkEventTimeValue(strconv.FormatInt(v, 10)); !ts.IsZero() {
				return ts
			}
		case float64:
			if ts := parseDingTalkEventTimeValue(strconv.FormatInt(int64(v), 10)); !ts.IsZero() {
				return ts
			}
		}
	}
	return time.Time{}
}

func dingtalkRawPayloadLogEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("YAK_IM_DEBUG_RAW"))) {
	case "1", "true", "yes", "on", "dingtalk":
		return true
	default:
		return false
	}
}

func parseDingTalkEventTimeValue(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return time.Time{}
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		switch {
		case n > 1e12:
			return time.UnixMilli(n)
		case n > 1e9:
			return time.Unix(n, 0)
		}
	}
	if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return ts
	}
	return time.Time{}
}

// replyContext 是钉钉 InboundMessage.ReplyContext 的实际类型。
// 优先用 SessionWebhook 回复（轻量，无需 token）；为空时回退到主动发送（需 access_token）。
// MsgID/RobotCode/ConversationID 用于上下文和 emotion API（给消息加自定义表情）。
type replyContext struct {
	SessionWebhook string
	ConversationID string
	SenderStaffID  string
	MsgID          string
	RobotCode      string
	IsGroup        bool
}

// normalizeDingTalkChatType 把钉钉 conversationType 归一化为跨平台统一取值。
// 钉钉 conversationType: "1"=单聊, "2"=群。
func normalizeDingTalkChatType(conversationType string) string {
	switch conversationType {
	case "1":
		return "private"
	case "2":
		return "group"
	}
	return conversationType
}

// extractSenderID 从 senderId 的 json.RawMessage 提取发送者 ID。
// 兼容钉钉两种格式：字符串（"staffId123"）或对象（{"staffId":"...","unionId":"..."}）。
func extractSenderID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// 尝试当字符串解析
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	// 尝试当对象解析
	var obj struct {
		StaffID string `json:"staffId"`
		UnionID string `json:"unionId"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		if obj.StaffID != "" {
			return obj.StaffID
		}
		return obj.UnionID
	}
	return ""
}

// writeACK 写回 ACK DataFrameResponse JSON。
func (c *Client) writeACK(client *lowhttp.WebsocketClient, messageID string, data any) {
	if client == nil {
		return
	}
	resp := frameResponse{Code: 200, Message: "OK"}
	resp.Headers.ContentType = "application/json"
	resp.Headers.MessageID = messageID
	resp.Data = data
	b, _ := json.Marshal(resp)
	if err := client.WriteText(b); err != nil {
		log.Debugf("dingtalk: write ack failed: %v", err)
	}
}

// buildWSUpgradePacket 把 ws/wss URL 渲染成一个 ws 升级 HTTP GET 报文。
func buildWSUpgradePacket(wsURL string) []byte {
	// lowhttp.ParseUrlToHttpRequestRaw 期望 http(s) URL，这里把 scheme 归一化。
	httpURL := strings.Replace(wsURL, "ws://", "http://", 1)
	httpURL = strings.Replace(httpURL, "wss://", "https://", 1)
	isHTTPS, req, err := lowhttp.ParseUrlToHttpRequestRaw("GET", httpURL)
	_ = isHTTPS
	if err != nil {
		// 兜底：手动拼一个最小 GET（不应发生）。
		req = []byte("GET " + httpURL + " HTTP/1.1\r\nHost: \r\n\r\n")
	}
	// 补全 ws 升级头（lowhttp 解析出的报文可能不含这些）。
	req = lowhttp.ReplaceHTTPPacketHeader(req, "Upgrade", "websocket")
	req = lowhttp.ReplaceHTTPPacketHeader(req, "Connection", "Upgrade")
	if string(lowhttp.GetHTTPPacketHeader(req, "Sec-WebSocket-Version")) == "" {
		req = lowhttp.ReplaceHTTPPacketHeader(req, "Sec-WebSocket-Version", "13")
	}
	if string(lowhttp.GetHTTPPacketHeader(req, "Sec-WebSocket-Key")) == "" {
		req = lowhttp.ReplaceHTTPPacketHeader(req, "Sec-WebSocket-Key", genSecWebSocketKey())
	}
	return req
}

// genSecWebSocketKey 生成 16 字节随机数的 base64 编码，作为 Sec-WebSocket-Key。
func genSecWebSocketKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "yaklang-notify-default-key=" // 兜底（不应发生）
	}
	return base64.StdEncoding.EncodeToString(b)
}
