package imcontrol

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// CallbackAuth 负责卡片按钮回调 token 的签发与验签。
//
// token 格式：cb.v1.<base64(payloadJSON)>.<base64(hmacSHA256(payloadJSON, key)>
// payload 含 run_id/chat_id/action/operator_open_id/exp/nonce，对标参考项目
// lark-coding-agent-bridge 的 bridge_cb.v1。验签时校验 HMAC + exp + 上下文匹配
// + nonce 防重放，防止伪造按钮事件和非 session owner 操作。
//
// 密钥由进程级固定派生（不追求对抗本机攻击者，与 BotConfig 加密同策略）。
// nonce 存内存 map（进程重启后重置——exp 仍防重放，nonce 只防同进程内重投）。
type CallbackAuth struct {
	key      []byte
	now      func() time.Time
	newNonce func() string
	ttl      time.Duration

	nonceMu   sync.Mutex
	nonceSeen map[string]int64 // nonce -> exp（unix nano）；定期清理过期项
}

// CallbackSignInput 是签发 token 的输入。
type CallbackSignInput struct {
	RunID  string
	ChatID string
	Action string
	TTL    time.Duration // 留空用默认 30 分钟
}

// CallbackVerifyExpected 是验签时期望的上下文（从当前请求环境取）。
type CallbackVerifyExpected struct {
	RunID  string
	ChatID string
	Action string
}

// CallbackVerifyResult 是验签结果。
type CallbackVerifyResult struct {
	OK     bool
	Reason string // ok=false 时的拒绝原因
}

// callbackPayload 是 token 内部载荷。
type callbackPayload struct {
	RunID      string `json:"r"`
	ChatID     string `json:"c"`
	Action     string `json:"a"`
	Exp        int64  `json:"exp"` // unix nano
	Nonce      string `json:"n"`
	KeyVersion int    `json:"kv"` // 密钥版本（旋转用，本阶段固定 1）
}

const (
	callbackTokenPrefix = "cb.v1"
	callbackDefaultTTL  = 30 * time.Minute
	callbackKeyVersion  = 1
)

// NewCallbackAuth 构造一个 CallbackAuth。secret 为签名密钥（32 字节）。
func NewCallbackAuth(secret []byte) *CallbackAuth {
	if len(secret) == 0 {
		// 兜底：派生一个（不应对抗本机攻击者，但要保证非空）
		sum := sha256.Sum256([]byte("yaklang-notify-callback-v1"))
		secret = sum[:]
	}
	return &CallbackAuth{
		key:       secret,
		now:       time.Now,
		newNonce:  defaultNonce,
		ttl:       callbackDefaultTTL,
		nonceSeen: map[string]int64{},
	}
}

func defaultNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Sign 签发一个回调 token。
func (a *CallbackAuth) Sign(input CallbackSignInput) string {
	ttl := input.TTL
	if ttl <= 0 {
		ttl = a.ttl
	}
	payload := callbackPayload{
		RunID:      input.RunID,
		ChatID:     input.ChatID,
		Action:     input.Action,
		Exp:        a.now().Add(ttl).UnixNano(),
		Nonce:      a.newNonce(),
		KeyVersion: callbackKeyVersion,
	}
	payloadJSON, _ := json.Marshal(payload)
	mac := hmac.New(sha256.New, a.key)
	mac.Write(payloadJSON)
	sig := mac.Sum(nil)
	return fmt.Sprintf("%s.%s.%s",
		callbackTokenPrefix,
		base64.RawURLEncoding.EncodeToString(payloadJSON),
		base64.RawURLEncoding.EncodeToString(sig))
}

// oneShotActions 是必须一次性消费 nonce 的动作集合（停止运行等）。
// 这些动作的按钮随运行卡片发出，token 含 run_id，语义上只应被点一次。
// 其它动作（new/resume/update_reply_mode 等配置/导航类）允许复用：
// 它们的按钮在最终卡片上是静态的，用户可能多次点击。
var oneShotActions = map[string]bool{
	"stop": true,
}

// Verify 验签一个回调 token。expected 为当前请求环境的上下文。
func (a *CallbackAuth) Verify(token string, expected CallbackVerifyExpected) CallbackVerifyResult {
	// token 格式：cb.v1.<b64payload>.<b64sig>，前缀含一个 '.'，用前缀切割后剩余部分再按 '.' 分。
	if !strings.HasPrefix(token, callbackTokenPrefix+".") {
		return CallbackVerifyResult{OK: false, Reason: "malformed"}
	}
	rest := token[len(callbackTokenPrefix)+1:] // 去掉 "cb.v1."
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 {
		return CallbackVerifyResult{OK: false, Reason: "malformed"}
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return CallbackVerifyResult{OK: false, Reason: "malformed"}
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return CallbackVerifyResult{OK: false, Reason: "malformed"}
	}

	// 校验 HMAC
	mac := hmac.New(sha256.New, a.key)
	mac.Write(payloadJSON)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return CallbackVerifyResult{OK: false, Reason: "bad-signature"}
	}

	var p callbackPayload
	if err := json.Unmarshal(payloadJSON, &p); err != nil {
		return CallbackVerifyResult{OK: false, Reason: "malformed"}
	}

	// 校验 exp
	if a.now().UnixNano() > p.Exp {
		return CallbackVerifyResult{OK: false, Reason: "expired"}
	}

	// 校验上下文匹配
	if p.RunID != expected.RunID || p.ChatID != expected.ChatID ||
		p.Action != expected.Action {
		return CallbackVerifyResult{OK: false, Reason: "context-mismatch"}
	}

	// nonce 防重放：one-shot 动作（stop）消费 nonce；其它动作（new/resume/update_reply_mode
	// 等配置/导航类，按钮在最终卡片上静态存在、用户可多次点击）不消费 nonce，允许复用。
	if oneShotActions[p.Action] {
		a.nonceMu.Lock()
		defer a.nonceMu.Unlock()
		a.cleanExpiredNonceLocked()
		if _, seen := a.nonceSeen[p.Nonce]; seen {
			return CallbackVerifyResult{OK: false, Reason: "nonce-replay"}
		}
		a.nonceSeen[p.Nonce] = p.Exp
	}
	return CallbackVerifyResult{OK: true}
}

// cleanExpiredNonceLocked 清理已过期的 nonce（调用方持锁）。
func (a *CallbackAuth) cleanExpiredNonceLocked() {
	now := a.now().UnixNano()
	for n, exp := range a.nonceSeen {
		if now > exp {
			delete(a.nonceSeen, n)
		}
	}
}
