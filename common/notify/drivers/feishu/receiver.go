package feishu

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/internal/httpclient"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// imMessagePayload 飞书 im.message.receive_v1 事件的 payload（精简）。
type imMessagePayload struct {
	Event struct {
		Sender struct {
			SenderID struct {
				OpenID  string `json:"open_id"`
				UserID  string `json:"user_id"`
				UnionID string `json:"union_id"`
			} `json:"sender_id"`
			SenderType string `json:"sender_type"`
		} `json:"sender"`
		Message struct {
			MessageID  string          `json:"message_id"`
			ChatID     string          `json:"chat_id"`
			ChatType   string          `json:"chat_type"` // p2p / group
			MsgType    string          `json:"message_type"`
			CreateTime any             `json:"create_time"`
			Content    json.RawMessage `json:"content"` // 飞书通常是 JSON 字符串，部分客户端图文混发可能是对象/数组
			// RootID 根消息 ID，仅在回复消息场景返回。
			RootID string `json:"root_id"`
			// ParentID 父消息 ID，仅在回复消息场景返回。
			ParentID string `json:"parent_id"`
			// ThreadID 消息所属话题 ID，仅话题消息返回；非话题消息不返回。
			ThreadID string `json:"thread_id"`
			Mentions []struct {
				Key string `json:"key"`
				ID  struct {
					OpenID  string `json:"open_id"`
					UserID  string `json:"user_id"`
					UnionID string `json:"union_id"`
				} `json:"id"`
				Name     string `json:"name"`
				TenantID string `json:"tenant_key"`
			} `json:"mentions"`
		} `json:"message"`
	} `json:"event"`
}

// Start 启动飞书 Long Connection 接收：获取 ws 地址 → 建 ws → 读 binary Frame + 心跳 + ACK + 断线重连。
//
// 阻塞直到 ctx 取消或不可恢复错误。
func (c *Client) Start(ctx context.Context, handler func(*notify.InboundMessage)) error {
	cfg := c.loadConfig()
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return fmt.Errorf("feishu: app_id/app_secret required for long connection")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := c.runLCOnce(ctx, cfg, handler)
		if err == nil {
			err = errors.New("connection closed; reconnecting")
		}
		c.emitEvent(notify.Event{
			Type:     notify.EventError,
			Platform: notify.PlatformFeishu,
			Err:      err,
		})
		if isExpectedLCDisconnect(err) {
			log.Debugf("feishu: long connection disconnected, reconnecting: %v", err)
		} else {
			log.Warnf("feishu: long connection disconnected: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

// runLCOnce 建立一次完整 LC 会话。
func (c *Client) runLCOnce(ctx context.Context, cfg *notify.SendConfig, handler func(*notify.InboundMessage)) error {
	wsURL, pingInterval, err := c.fetchWSEndpoint(ctx, cfg)
	if err != nil {
		return fmt.Errorf("fetch ws endpoint: %w", err)
	}
	return c.runWS(ctx, wsURL, cfg, pingInterval, handler)
}

// fetchWSEndpoint POST /callback/ws/endpoint 拿 ws 地址与心跳间隔。
// 端点路径用单数 endpoint（官方 SDK ws/const.go: GenEndpointUri = "/callback/ws/endpoint"）。
func (c *Client) fetchWSEndpoint(ctx context.Context, cfg *notify.SendConfig) (wsURL string, pingInterval time.Duration, err error) {
	url := c.base() + "/callback/ws/endpoint"
	headers := jsonHeaders()
	headers["locale"] = "zh"
	result, rerr := httpclient.Do("POST", url, headers, nil,
		map[string]string{"AppID": cfg.AppID, "AppSecret": cfg.AppSecret},
		buildHTTPOpts(cfg.Proxy, cfg.Timeout)...)
	if rerr != nil {
		return "", 0, rerr
	}
	if result.StatusCode != 200 {
		return "", 0, fmt.Errorf("ws endpoint status %d: %s", result.StatusCode, string(result.Body))
	}
	var resp EndpointResp
	if err := json.Unmarshal(result.Body, &resp); err != nil {
		return "", 0, fmt.Errorf("decode ws endpoint: %w", err)
	}
	if resp.Code != 0 || resp.Data == nil || resp.Data.Url == "" {
		return "", 0, fmt.Errorf("ws endpoint failed code=%d msg=%s", resp.Code, resp.Msg)
	}
	ping := 2 * time.Minute // 飞书默认心跳间隔
	if resp.Data.ClientConfig != nil && resp.Data.ClientConfig.PingInterval > 0 {
		ping = time.Duration(resp.Data.ClientConfig.PingInterval) * time.Second
	}
	return resp.Data.Url, ping, nil
}

// runWS 连接 ws 并处理 binary frame。
func (c *Client) runWS(ctx context.Context, wsURL string, cfg *notify.SendConfig, pingInterval time.Duration, handler func(*notify.InboundMessage)) error {
	packet := buildWSUpgradePacket(wsURL)
	isTLS := strings.HasPrefix(wsURL, "wss://")
	// 从 ws URL 解析 service_id，ping 帧需要带上（官方 SDK 同样做法）。
	serviceID := int32(0)
	if u, err := neturl.Parse(wsURL); err == nil {
		if sid := u.Query().Get("service_id"); sid != "" {
			if v, err := strconv.Atoi(sid); err == nil {
				serviceID = int32(v)
			}
		}
	}
	wsOpts := []lowhttp.WebsocketClientOpt{
		lowhttp.WithWebsocketWithContext(ctx),
		lowhttp.WithWebsocketTLS(isTLS),
		lowhttp.WithWebsocketTotalTimeout(0),
	}
	if cfg.Proxy != "" {
		wsOpts = append(wsOpts, lowhttp.WithWebsocketProxy(cfg.Proxy))
	}
	wsOpts = append(wsOpts, lowhttp.WithWebsocketAllFrameHandler(func(client *lowhttp.WebsocketClient, frame *lowhttp.Frame, plain []byte, shutdown func()) {
		if frame == nil {
			return
		}
		// 仅处理 binary 帧（飞书 LC 业务帧）。
		if frame.Type() != lowhttp.BinaryMessage {
			// 标准 ping 由 lowhttp 自动回 pong，这里忽略其它控制帧。
			return
		}
		// plain 是解码后的帧净荷，优先用 plain；为空时回退 frame.GetData()。
		data := plain
		if len(data) == 0 {
			data = frame.GetData()
		}
		c.handleLCFrame(data, client, handler)
	}))

	client, err := lowhttp.NewWebsocketClient(packet, wsOpts...)
	if err != nil {
		return fmt.Errorf("ws dial: %w", err)
	}
	client.Start()
	c.emitEvent(notify.Event{
		Type:     notify.EventConnected,
		Platform: notify.PlatformFeishu,
	})

	// 心跳：按服务端下发的 PingInterval 发 LC PingFrame 保活，对齐官方 SDK pingLoop
	// （oapi-sdk-go/ws/client.go:496-519，用原值不折半）。官方默认 2 分钟。
	// lowhttp.WebsocketClient 的 WriteBinary 无锁，ping 与 ACK 已通过 wsWriteMu 串行化。
	effectivePing := pingInterval
	if effectivePing < 30*time.Second {
		effectivePing = 30 * time.Second
	}
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(effectivePing)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				c.writePing(client, serviceID)
			}
		}
	}()

	// P3-6: 低频清理合包缓存，避免丢包导致 combineCache 长期积累（每 30 秒清一次）。
	// 分包通常在毫秒级到达，30 秒还没收齐的视为丢包。
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				c.reapCombineCache()
			}
		}
	}()

	defer close(stop)

	<-client.Context.Done()
	err = client.Close()
	if isExpectedLCDisconnect(err) {
		return nil
	}
	return err
}

func isExpectedLCDisconnect(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "use of closed connection") ||
		strings.Contains(msg, "use of closed network connection")
}

// handleLCFrame 解析一个飞书 LC binary frame（protobuf 编码），回 ACK，并回调入站事件。
// 对齐官方 SDK oapi-sdk-go/ws/client.go:597-685 的合包逻辑：
//   - Data 帧的 Headers 带 sum/seq/message_id；sum>1 表示飞书把大消息拆成多包，
//     需按 message_id 缓存分包、收齐所有 seq 后拼接成完整 payload 再 dispatch。
func (c *Client) handleLCFrame(data []byte, client *lowhttp.WebsocketClient, handler func(*notify.InboundMessage)) {
	f := &Frame{}
	if err := f.Unmarshal(data); err != nil {
		log.Debugf("feishu: drop malformed frame: %v", err)
		return
	}
	switch FrameType(f.Method) {
	case FrameTypeControl:
		// 控制帧（如 Pong），无需业务处理。
		return
	case FrameTypeData:
		// 合包：sum>1 时按 message_id 缓存分包，收齐后拼接。
		hs := Headers(f.Headers)
		sum := hs.GetInt(HeaderSum)
		// 按 LC frame header `type` 分流：event=消息事件，card=卡片按钮回调。
		// 两种都走合包 + ACK，卡片回调同样需要 ACK 否则飞书会重投。
		frameType := hs.GetString(HeaderType)
		if sum > 1 {
			msgID := hs.GetString(HeaderMessageID)
			seq := hs.GetInt(HeaderSeq)
			if msgID == "" {
				log.Debugf("feishu: data frame sum>1 but message_id empty, drop")
				return
			}
			pl := c.combineFrame(msgID, sum, seq, f.Payload)
			if pl == nil {
				// 还没收齐所有分包，不回 ACK（等齐了再回）
				return
			}
			// 收齐后用完整 payload 构造合成帧。ACK 必须先回给飞书，避免业务处理慢导致重投或按钮超时。
			merged := *f
			merged.Payload = pl
			c.writeACK(client, &merged)
			c.dispatchByType(frameType, &merged, handler)
			return
		}
		// sum<=1（未拆包）：先 ACK 再分发。handler 可能触发 AI/卡片 patch，不能阻塞平台 ACK。
		c.writeACK(client, f)
		c.dispatchByType(frameType, f, handler)
	}
}

// dispatchByType 按 payload event_type 优先识别卡片回调，再按 LC frame header 的 type 兜底分流。
// 飞书长连接里 card.action.trigger 可能以 type=event 下发，不能只依赖 frame header type。
func (c *Client) dispatchByType(frameType string, f *Frame, handler func(*notify.InboundMessage)) {
	if isCardActionPayload(f) {
		log.Infof("feishu: receive card action payload")
		c.dispatchCardAction(f, handler)
		return
	}
	switch MessageType(frameType) {
	case MessageTypeCard:
		log.Infof("feishu: receive card frame")
		c.dispatchCardAction(f, handler)
	default: // event / 空（老版本 payload 不带 type，按消息事件兜底）
		c.dispatchEvent(f, handler)
	}
}

func isCardActionPayload(f *Frame) bool {
	if f == nil || len(f.Payload) == 0 {
		return false
	}
	var payload struct {
		Header struct {
			EventType string `json:"event_type"`
		} `json:"header"`
	}
	if err := json.Unmarshal(f.Payload, &payload); err == nil && payload.Header.EventType == "card.action.trigger" {
		return true
	}
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(f.Payload, &wrapper); err != nil || len(wrapper.Data) == 0 {
		return false
	}
	if err := json.Unmarshal(wrapper.Data, &payload); err != nil {
		return false
	}
	return payload.Header.EventType == "card.action.trigger"
}

func extractFeishuRawMessage(payload []byte) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil
	}
	if msg := feishuMessageMapFromRoot(root); msg != nil {
		return msg
	}
	if data, ok := root["data"].(map[string]any); ok {
		return feishuMessageMapFromRoot(data)
	}
	return nil
}

func feishuMessageMapFromRoot(root map[string]any) map[string]any {
	event, ok := root["event"].(map[string]any)
	if !ok {
		return nil
	}
	msg, ok := event["message"].(map[string]any)
	if !ok {
		return nil
	}
	return msg
}

func compactJSONString(value any, maxLen int) string {
	b, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	s := string(b)
	if maxLen > 0 && len(s) > maxLen {
		return s[:maxLen] + "...(truncated)"
	}
	return s
}

func feishuContentString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
}

// combineFrame 按 message_id+seq 缓存分包，收齐所有 seq 后返回拼接完整的 payload。
// 对齐官方 SDK oapi-sdk-go/ws/client.go:659-685 的 combine 实现。
// 返回 nil 表示尚未收齐；返回非 nil 表示合包完成（按 seq 顺序拼接）。
func (c *Client) combineFrame(msgID string, sum, seq int, bs []byte) []byte {
	// P2-5: 边界检查，异常 seq/sum 直接丢弃，避免越界 panic 拖垮长连接 goroutine。
	if sum <= 0 || seq < 0 || seq >= sum {
		log.Warnf("feishu: drop out-of-range frame: msgID=%s sum=%d seq=%d", msgID, sum, seq)
		return nil
	}
	c.combineMu.Lock()
	defer c.combineMu.Unlock()
	val, ok := c.combineCache[msgID]
	if !ok {
		buf := make([][]byte, sum)
		buf[seq] = bs
		c.combineCache[msgID] = buf
		return nil
	}
	// 缓存里的 buf 长度可能与本次 sum 不一致（异常情况），做防御
	if len(val) <= seq {
		log.Warnf("feishu: drop frame seq out of cached range: msgID=%s cachedLen=%d seq=%d", msgID, len(val), seq)
		return nil
	}
	val[seq] = bs
	// 检查是否所有分包都已收齐
	capacity := 0
	for _, v := range val {
		if len(v) == 0 {
			return nil // 还有缺口
		}
		capacity += len(v)
	}
	// 收齐：按 seq 顺序拼接
	pl := make([]byte, 0, capacity)
	for _, v := range val {
		pl = append(pl, v...)
	}
	delete(c.combineCache, msgID)
	return pl
}

// reapCombineCache 定期清理过期的合包缓存，避免丢包导致内存泄漏。
func (c *Client) reapCombineCache() {
	c.combineMu.Lock()
	defer c.combineMu.Unlock()
	// 简单策略：每次调用清空所有未完成的缓存（由 caller 定时触发）。
	// 更精细的 TTL 机制不必要——飞书分包通常在毫秒级到达。
	for k := range c.combineCache {
		delete(c.combineCache, k)
	}
}

// dispatchEvent 把飞书事件 payload 解析成 InboundMessage 回调。
func (c *Client) dispatchEvent(f *Frame, handler func(*notify.InboundMessage)) {
	if handler == nil || len(f.Payload) == 0 {
		return
	}
	// payload 是事件 JSON（通常 {event:{...}} 或带 schema 外层）。
	var payload imMessagePayload
	if err := json.Unmarshal(f.Payload, &payload); err != nil {
		// 某些飞书事件 payload 是带 "data"/"event" 外层；尝试两种。
		var wrapper struct {
			Data json.RawMessage `json:"data"`
		}
		if err2 := json.Unmarshal(f.Payload, &wrapper); err2 == nil && len(wrapper.Data) > 0 {
			if err3 := json.Unmarshal(wrapper.Data, &payload); err3 != nil {
				return
			}
		} else {
			return
		}
	}
	msg := payload.Event.Message
	if msg.MessageID == "" {
		return
	}
	content := feishuContentString(msg.Content)
	text, attachments := extractFeishuContent(content, msg.MsgType, msg.MessageID)
	if text == "" && len(attachments) == 0 {
		if rawMsg := extractFeishuRawMessage(f.Payload); rawMsg != nil {
			text, attachments = extractGenericFeishuContent(rawMsg, msg.MessageID)
			if text == "" && len(attachments) == 0 {
				log.Warnf("feishu: parsed empty message content message_id=%s type=%s content_len=%d raw_message=%s",
					msg.MessageID, msg.MsgType, len(content), compactJSONString(rawMsg, 512))
			}
		}
	}
	handler(&notify.InboundMessage{
		Platform:     notify.PlatformFeishu,
		ChatID:       msg.ChatID,
		SenderID:     payload.Event.Sender.SenderID.OpenID,
		SenderName:   "",
		Text:         text,
		EventTime:    feishuInboundEventTime(msg.CreateTime, f),
		Raw:          f.Payload,
		ReplyContext: msg.MessageID,
		ChatType:     normalizeFeishuChatType(msg.ChatType, msg.ThreadID),
		ThreadID:     msg.ThreadID,
		RootID:       msg.RootID,
		ParentID:     msg.ParentID,
		Attachments:  attachments,
		MentionBot:   len(msg.Mentions) > 0,
	})
}

// normalizeFeishuChatType 把飞书原始 chat_type 归一化为跨平台统一取值。
// 飞书 chat_type 取值只有 p2p / group；话题群在 chat_type 上仍是 group，
// 靠 thread_id 是否存在二次区分。
//   - p2p                    -> private
//   - group（无 thread_id）  -> group
//   - group（带 thread_id）  -> topic
func normalizeFeishuChatType(chatType, threadID string) string {
	switch chatType {
	case "p2p":
		return "private"
	case "group":
		if threadID != "" {
			return "topic"
		}
		return "group"
	}
	return chatType
}

// cardActionPayload 飞书卡片按钮回调事件（card.action.trigger）的 payload。
// schema 2.0 下按钮用 behaviors:[{type:"callback", value:...}] 声明，
// 点击后飞书经长连接推送本 payload，event.action.value 即按钮 value 原样回传。
type cardActionPayload struct {
	Schema string `json:"schema"` // "2.0"
	Header struct {
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"` // "card.action.trigger"
		Token     string `json:"token"`
		AppID     string `json:"app_id"`
	} `json:"header"`
	Event struct {
		Operator struct {
			OpenID  string `json:"open_id"`
			UserID  string `json:"user_id"`
			UnionID string `json:"union_id"`
			// 飞书部分事件把操作者 ID 放在 operator.operator_id 下。
			OperatorID struct {
				OpenID  string `json:"open_id"`
				UserID  string `json:"user_id"`
				UnionID string `json:"union_id"`
			} `json:"operator_id"`
		} `json:"operator"`
		Action struct {
			Value  map[string]any `json:"value"`  // 按钮 payload，渲染时塞的 value 原样回传
			Tag    string         `json:"tag"`    // "button"
			Option string         `json:"option"` // select_static 选中的 options.value
		} `json:"action"`
		Context struct {
			OpenMessageID string `json:"open_message_id"`
			OpenChatID    string `json:"open_chat_id"`
		} `json:"context"`
		// 承载卡片的消息 ID（用于 patch 更新该卡片）。飞书两套字段名兜底。
		MessageID     string `json:"message_id"`
		OpenMessageID string `json:"open_message_id"`
		ChatID        string `json:"chat_id"`
		OpenChatID    string `json:"open_chat_id"`
	} `json:"event"`
}

// dispatchCardAction 解析卡片按钮回调事件，构造 InboundMessage 上报上层。
// 上层（imcontrol.handleMessage）通过 IsCardAction=true 判定走 IMAction 分支
// 而非喂给 agent。ReplyContext 设为卡片消息 ID，供 presenter patch 更新该卡片。
func (c *Client) dispatchCardAction(f *Frame, handler func(*notify.InboundMessage)) {
	var payload cardActionPayload
	if err := json.Unmarshal(f.Payload, &payload); err != nil {
		// 部分老版本 payload 嵌在 {data:...} wrapper 里
		var wrapper struct {
			Data json.RawMessage `json:"data"`
		}
		if e2 := json.Unmarshal(f.Payload, &wrapper); e2 == nil && len(wrapper.Data) > 0 {
			if err2 := json.Unmarshal(wrapper.Data, &payload); err2 != nil {
				log.Debugf("feishu: drop unparseable card action frame: %v", err2)
				return
			}
		} else {
			log.Debugf("feishu: drop unparseable card action frame: %v", err)
			return
		}
	}

	ev := payload.Event
	chatID := ev.ChatID
	if chatID == "" {
		chatID = ev.OpenChatID
	}
	if chatID == "" {
		chatID = ev.Context.OpenChatID
	}
	msgID := ev.MessageID
	if msgID == "" {
		msgID = ev.OpenMessageID
	}
	if msgID == "" {
		msgID = ev.Context.OpenMessageID
	}
	if len(ev.Action.Value) == 0 {
		log.Debugf("feishu: card action frame has empty action.value, drop")
		return
	}
	actionValue := ev.Action.Value
	if ev.Action.Option != "" {
		actionValue = make(map[string]any, len(ev.Action.Value)+1)
		for k, v := range ev.Action.Value {
			actionValue[k] = v
		}
		actionValue["option"] = ev.Action.Option
	}
	actionStr, _ := actionValue["action"].(string)
	operatorOpenID := ev.Operator.OpenID
	if operatorOpenID == "" {
		operatorOpenID = ev.Operator.OperatorID.OpenID
	}
	log.Infof("feishu: dispatch card action action=%s chat=%s message=%s operator=%s", actionStr, chatID, msgID, operatorOpenID)
	handler(&notify.InboundMessage{
		Platform:     notify.PlatformFeishu,
		ChatID:       chatID,
		SenderID:     operatorOpenID,
		Text:         "",
		EventTime:    feishuEventTime(f),
		Raw:          f.Payload,
		ReplyContext: msgID, // 卡片消息 ID，presenter 用它 patch 该卡片
		IsCardAction: true,
		ActionValue:  actionValue,
	})
}

func feishuEventTime(f *Frame) time.Time {
	if f == nil {
		return time.Time{}
	}
	return parseFeishuEventTimeValue(Headers(f.Headers).GetString(HeaderTimestamp))
}

func feishuInboundEventTime(createTime any, f *Frame) time.Time {
	if ts := parseFeishuEventTimeValue(createTime); !ts.IsZero() {
		return ts
	}
	return feishuEventTime(f)
}

func parseFeishuEventTimeValue(value any) time.Time {
	var raw string
	switch v := value.(type) {
	case nil:
		return time.Time{}
	case string:
		raw = v
	case int64:
		raw = strconv.FormatInt(v, 10)
	case int:
		raw = strconv.Itoa(v)
	case float64:
		raw = strconv.FormatInt(int64(v), 10)
	case json.Number:
		raw = v.String()
	default:
		raw = fmt.Sprint(v)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return time.Time{}
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err == nil {
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

// extractFeishuContent 从飞书消息 content(JSON 字符串) 提取纯文本和附件。
// image/file 消息产 IMAttachment（含 image_key/file_key/message_id），text 为空或描述。
// post 消息提取嵌套文本 + img 的 image_key。
func extractFeishuContent(content, msgType, messageID string) (string, []notify.IMAttachment) {
	if content == "" {
		return "", nil
	}
	var m map[string]any
	if json.Unmarshal([]byte(content), &m) != nil {
		return content, nil // 非 JSON 回退原文
	}
	switch msgType {
	case "text":
		if t, ok := m["text"].(string); ok {
			return t, nil
		}
		return extractGenericFeishuContent(m, messageID)
	case "image":
		imageKey, _ := m["image_key"].(string)
		if imageKey == "" {
			return extractGenericFeishuContent(m, messageID)
		}
		return "", []notify.IMAttachment{{
			Type:      notify.MsgImage,
			FileKey:   imageKey,
			MessageID: messageID,
		}}
	case "file":
		fileKey, _ := m["file_key"].(string)
		fileName, _ := m["file_name"].(string)
		if fileKey == "" {
			return extractGenericFeishuContent(m, messageID)
		}
		return "", []notify.IMAttachment{{
			Type:      notify.MsgFile,
			FileKey:   fileKey,
			FileName:  fileName,
			MessageID: messageID,
		}}
	case "post":
		// post 富文本：提取文本 + 嵌套 img 的 image_key
		return extractPostContent(m, messageID)
	default:
		text, attachments := extractGenericFeishuContent(m, messageID)
		if text != "" || len(attachments) > 0 {
			return text, attachments
		}
		return content, nil
	}
}

// extractPostContent 从飞书 post 富文本提取文本和嵌套图片附件。
// post content 格式：{"zh_cn":{"title":"...","content":[[{"tag":"text","text":"..."},{"tag":"img","image_key":"..."}]]}}
func extractPostContent(m map[string]any, messageID string) (string, []notify.IMAttachment) {
	// post 既可能是旧版多语言结构：
	// {"zh_cn":{"title":"...","content":[...]}}
	// 也可能直接是富文本正文：
	// {"title":"","content":[[{"tag":"img","image_key":"..."}]]}
	lang := m
	if _, ok := lang["content"].([]any); !ok {
		lang = nil
		for _, v := range m {
			if lv, ok := v.(map[string]any); ok {
				if _, hasContent := lv["content"].([]any); hasContent {
					lang = lv
					break
				}
			}
		}
	}
	if lang == nil {
		return extractGenericFeishuContent(m, messageID)
	}
	var sb strings.Builder
	var attachments []notify.IMAttachment
	if title, ok := lang["title"].(string); ok && title != "" {
		sb.WriteString(title + "\n")
	}
	contentArr, ok := lang["content"].([]any)
	if !ok {
		return strings.TrimSpace(sb.String()), nil
	}
	for _, row := range contentArr {
		paragraph, ok := row.([]any)
		if !ok {
			continue
		}
		for _, elem := range paragraph {
			tag, _ := elem.(map[string]any)
			if tag == nil {
				continue
			}
			switch tag["tag"] {
			case "text":
				if text, ok := tag["text"].(string); ok {
					sb.WriteString(text)
				}
			case "img":
				if imgKey, ok := tag["image_key"].(string); ok && imgKey != "" {
					attachments = append(attachments, notify.IMAttachment{
						Type: notify.MsgImage, FileKey: imgKey, MessageID: messageID,
					})
				}
			case "a":
				if text, ok := tag["text"].(string); ok {
					sb.WriteString(text)
				}
			}
		}
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String()), attachments
}

func extractGenericFeishuContent(value any, messageID string) (string, []notify.IMAttachment) {
	var sb strings.Builder
	attachments := make([]notify.IMAttachment, 0)
	seenImages := map[string]struct{}{}
	seenFiles := map[string]struct{}{}

	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if text := feishuStringValue(x, "text", "content", "title", "name"); text != "" {
				if nested, ok := parseFeishuNestedJSON(text); ok {
					walk(nested)
				} else {
					sb.WriteString(text)
					sb.WriteString("\n")
				}
			}
			if imageKey := feishuStringValue(x, "image_key", "imageKey", "file_key", "fileKey"); imageKey != "" && looksLikeFeishuImageKey(imageKey) {
				if _, ok := seenImages[imageKey]; !ok {
					seenImages[imageKey] = struct{}{}
					attachments = append(attachments, notify.IMAttachment{
						Type:      notify.MsgImage,
						FileKey:   imageKey,
						MessageID: messageID,
					})
				}
			}
			if fileKey := feishuStringValue(x, "file_key", "fileKey"); fileKey != "" && !looksLikeFeishuImageKey(fileKey) {
				if _, ok := seenFiles[fileKey]; !ok {
					seenFiles[fileKey] = struct{}{}
					attachments = append(attachments, notify.IMAttachment{
						Type:      notify.MsgFile,
						FileKey:   fileKey,
						FileName:  feishuStringValue(x, "file_name", "fileName", "name"),
						MessageID: messageID,
					})
				}
			}
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(value)
	return strings.TrimSpace(sb.String()), attachments
}

func parseFeishuNestedJSON(s string) (any, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	if !strings.HasPrefix(s, "{") && !strings.HasPrefix(s, "[") {
		return nil, false
	}
	var value any
	if err := json.Unmarshal([]byte(s), &value); err != nil {
		return nil, false
	}
	return value, true
}

func feishuStringValue(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key].(string); ok {
			v = strings.TrimSpace(v)
			if v != "" {
				return v
			}
		}
	}
	return ""
}

func looksLikeFeishuImageKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.HasPrefix(key, "img_") || strings.HasPrefix(key, "image_")
}

// writeACK 回一个业务 ACK：复用原 frame（保留 SeqID/LogID/Service 等关联字段），
// 把 Payload 替换成 Response{code:200} JSON。与官方 SDK handleDataFrame 一致。
func (c *Client) writeACK(client *lowhttp.WebsocketClient, origFrame *Frame) {
	if client == nil || origFrame == nil {
		return
	}
	resp := NewResponseByCode(200)
	p, err := json.Marshal(resp)
	if err != nil {
		log.Debugf("feishu: marshal ack response failed: %v", err)
		return
	}
	// 复制原 frame，保留飞书用于关联 ACK 的关键字段（SeqID/LogID/Service/Method/Headers）。
	ackFrame := *origFrame
	ackFrame.Payload = p
	b, err := ackFrame.Marshal()
	if err != nil {
		log.Debugf("feishu: marshal ack frame failed: %v", err)
		return
	}
	// 串行化写，避免与 ping 并发写损坏帧（对齐官方 SDK writeMessage 的 c.mu）。
	c.wsWriteMu.Lock()
	defer c.wsWriteMu.Unlock()
	if err := client.WriteBinary(b); err != nil {
		log.Debugf("feishu: write ack failed: %v", err)
	}
}

// writePing 发心跳 ping Frame（protobuf 编码），带上 serviceID。
func (c *Client) writePing(client *lowhttp.WebsocketClient, serviceID int32) {
	f := NewPingFrame(serviceID)
	b, err := f.Marshal()
	if err == nil {
		// 串行化写，避免与 ACK 并发写损坏帧（对齐官方 SDK writeMessage 的 c.mu）。
		c.wsWriteMu.Lock()
		_ = client.WriteBinary(b)
		c.wsWriteMu.Unlock()
	}
}

// buildWSUpgradePacket 渲染 ws 升级报文。
func buildWSUpgradePacket(wsURL string) []byte {
	httpURL := strings.Replace(wsURL, "ws://", "http://", 1)
	httpURL = strings.Replace(httpURL, "wss://", "https://", 1)
	_, req, err := lowhttp.ParseUrlToHttpRequestRaw("GET", httpURL)
	if err != nil {
		req = []byte("GET " + httpURL + " HTTP/1.1\r\nHost: \r\n\r\n")
	}
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

func genSecWebSocketKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "yaklang-notify-feishu-key="
	}
	return base64.StdEncoding.EncodeToString(b)
}
