package aibalance

import (
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// capability_probe.go 实现 aibalance Tool Calls Capability Matrix v1 的能力探测.
//
// 关键词: aibalance capability probe, tool_calls round1/round2 探测, ProbeToolCallsForProvider
//
// 设计契约:
//  1. 探测函数纯调用上游真实接口, 不修改 DB; 写 DB 由 SaveProbeResultByProviderID 完成.
//  2. round1 探测: 给上游发 tools=[probe_ping] + 一条要求模型调用工具的 user 消息,
//     若 onToolCall 回调被触发即视为 native; 否则 react.
//  3. round2 探测: 构造完整 round-trip messages (user / assistant.tool_calls / role=tool),
//     若上游返回的 stream 有任意非空字节即视为 native; 完全空回视为 react.
//  4. 每个探测独立 15s 超时, 失败不静默, 通过 probeErr 上报; 此时 mode 字段保留旧值,
//     避免误清空运维已配置的 mode.
//  5. 探测请求是"一次性"的, 不要走 RewriteMessagesForProviderInstance 等业务链路,
//     避免污染探测语义.

const (
	probeToolName        = "aibalance_probe_ping"
	probeToolDescription = "A probe tool used by aibalance to detect upstream tool_calls capability. When asked, you MUST call this tool with empty arguments {} and produce no other text."
	probeToolCallID      = "call_aibalance_probe_1"
	probeRoundTimeout    = 15 * time.Second
)

// ProbeResult 是一次完整 round1+round2 探测的结果摘要.
// 关键词: ProbeResult, 工具调用能力探测结果
type ProbeResult struct {
	Round1Mode string    `json:"round1_mode"` // "native" | "react"
	Round2Mode string    `json:"round2_mode"` // "native" | "react"
	ProbedAt   time.Time `json:"probed_at"`
	Error      string    `json:"error,omitempty"` // 探测过程中遇到的非致命错误; 为空表示完整完成
}

// ProbeToolCallsForProvider 对 Provider 真发两轮请求探测其工具调用兼容性.
// 关键词: ProbeToolCallsForProvider, capability probe entrypoint
func ProbeToolCallsForProvider(p *Provider) (*ProbeResult, error) {
	if p == nil {
		return nil, fmt.Errorf("provider is nil")
	}

	result := &ProbeResult{
		Round1Mode: "react",
		Round2Mode: "react",
		ProbedAt:   time.Now(),
	}

	// -------- round1: tools=[probe_ping], 期望上游回 tool_calls --------
	// 关键词: probe round1, native tool_calls detection
	r1Native, r1Err := probeRound1Native(p)
	if r1Err != nil {
		// 探测失败不直接放弃, 仍保留 react 作为保守默认, 同时把 err 累积上报
		result.Error = strings.TrimSpace("round1: " + r1Err.Error())
	}
	if r1Native {
		result.Round1Mode = "native"
	}

	// -------- round2: assistant.tool_calls + role=tool, 期望上游非空回复 --------
	// 关键词: probe round2, native NL detection
	r2Native, r2Err := probeRound2Native(p)
	if r2Err != nil {
		errStr := "round2: " + r2Err.Error()
		if result.Error == "" {
			result.Error = errStr
		} else {
			result.Error = result.Error + "; " + errStr
		}
	}
	if r2Native {
		result.Round2Mode = "native"
	}

	log.Infof("ProbeToolCallsForProvider: wrapper=%s model=%s type=%s round1=%s round2=%s err=%q",
		p.WrapperName, p.ModelName, p.TypeName, result.Round1Mode, result.Round2Mode, result.Error)

	return result, nil
}

// probeRound1Native 给上游发 tools=[probe_ping] 一次, 检测是否触发结构化 tool_calls.
// 关键词: probeRound1Native, native tool_calls detection
func probeRound1Native(p *Provider) (bool, error) {
	pingTool := aispec.Tool{
		Type: "function",
		Function: aispec.ToolFunction{
			Name:        probeToolName,
			Description: probeToolDescription,
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
		},
	}
	msgs := []aispec.ChatDetail{
		{
			Role:    "user",
			Content: "Call the aibalance_probe_ping tool now with empty arguments. Output nothing else.",
		},
	}

	var toolCallSeen int32
	client, err := p.GetAIClientWithRawMessages(
		msgs,
		[]aispec.Tool{pingTool},
		"auto",
		false,
		func(reader io.Reader) { _, _ = io.Copy(io.Discard, reader) },
		func(reader io.Reader) { _, _ = io.Copy(io.Discard, reader) },
		func(tcs []*aispec.ToolCall) {
			if len(tcs) > 0 {
				atomic.StoreInt32(&toolCallSeen, 1)
			}
		},
		nil,
	)
	if err != nil {
		return false, fmt.Errorf("failed to get ai client: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic: %v", r)
			}
		}()
		_, e := client.Chat("")
		done <- e
	}()

	select {
	case e := <-done:
		// 即便 Chat 报错, 只要回调期间已经看到 tool_calls, 也算 native
		if atomic.LoadInt32(&toolCallSeen) > 0 {
			return true, nil
		}
		if e != nil {
			return false, fmt.Errorf("chat failed: %v", e)
		}
		return false, nil
	case <-time.After(probeRoundTimeout):
		if atomic.LoadInt32(&toolCallSeen) > 0 {
			return true, nil
		}
		return false, fmt.Errorf("timeout after %s", probeRoundTimeout)
	}
}

// probeRound2Native 给上游发完整 round-trip messages, 检测是否能拿到非空 NL 响应.
// 关键词: probeRound2Native, native round-trip detection
func probeRound2Native(p *Provider) (bool, error) {
	msgs := []aispec.ChatDetail{
		{
			Role:    "user",
			Content: "Please call the aibalance_probe_ping tool and then summarize the result.",
		},
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []*aispec.ToolCall{
				{
					ID:   probeToolCallID,
					Type: "function",
					Function: aispec.FuncReturn{
						Name:      probeToolName,
						Arguments: "{}",
					},
				},
			},
		},
		{
			Role:       "tool",
			ToolCallID: probeToolCallID,
			Name:       probeToolName,
			Content:    `{"status":"ok","echo":"pong"}`,
		},
	}

	var contentBytes int64
	client, err := p.GetAIClientWithRawMessages(
		msgs,
		nil,
		nil,
		false,
		func(reader io.Reader) {
			buf := make([]byte, 1024)
			for {
				n, e := reader.Read(buf)
				if n > 0 {
					atomic.AddInt64(&contentBytes, int64(n))
				}
				if e != nil {
					return
				}
			}
		},
		func(reader io.Reader) { _, _ = io.Copy(io.Discard, reader) },
		nil,
		nil,
	)
	if err != nil {
		return false, fmt.Errorf("failed to get ai client: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic: %v", r)
			}
		}()
		_, e := client.Chat("")
		done <- e
	}()

	select {
	case e := <-done:
		if atomic.LoadInt64(&contentBytes) > 0 {
			return true, nil
		}
		if e != nil {
			return false, fmt.Errorf("chat failed: %v", e)
		}
		return false, nil
	case <-time.After(probeRoundTimeout):
		if atomic.LoadInt64(&contentBytes) > 0 {
			return true, nil
		}
		return false, fmt.Errorf("timeout after %s", probeRoundTimeout)
	}
}

// SaveProbeResultByProviderID 把 ProbeResult 落回数据库.
// 失败 / err 非空时 mode 字段保留旧值, 仅更新 ProbeAt + ProbeError.
// 关键词: SaveProbeResultByProviderID, capability probe persistence
func SaveProbeResultByProviderID(providerID uint, result *ProbeResult) error {
	if result == nil {
		return fmt.Errorf("probe result is nil")
	}
	dbProvider, err := GetAiProviderByID(providerID)
	if err != nil {
		return fmt.Errorf("failed to find provider id=%d: %v", providerID, err)
	}

	if result.Error == "" {
		// 探测完整完成 (无论 native / react), 全量覆写 mode
		dbProvider.ToolCallsRound1Mode = result.Round1Mode
		dbProvider.ToolCallsRound2Mode = result.Round2Mode
	}
	// 探测部分失败时, 保留 mode 旧值, 仅更新时间戳和错误信息
	dbProvider.ToolCallsProbeAt = result.ProbedAt
	dbProvider.ToolCallsProbeError = result.Error

	if err := UpdateAiProvider(dbProvider); err != nil {
		return fmt.Errorf("failed to update provider id=%d: %v", providerID, err)
	}
	log.Infof("SaveProbeResultByProviderID: id=%d round1=%s round2=%s probed_at=%s err=%q",
		providerID, dbProvider.ToolCallsRound1Mode, dbProvider.ToolCallsRound2Mode,
		result.ProbedAt.Format(time.RFC3339), result.Error)
	return nil
}

// ProbeAndSaveByProviderID 是一站式接口: 根据 ID 加载 Provider -> 探测 -> 落 DB.
// 关键词: ProbeAndSaveByProviderID, one-shot probe API
func ProbeAndSaveByProviderID(providerID uint) (*ProbeResult, error) {
	dbProvider, err := GetAiProviderByID(providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to find provider id=%d: %v", providerID, err)
	}
	p := dbAiProviderToRuntimeProvider(dbProvider)
	result, err := ProbeToolCallsForProvider(p)
	if err != nil {
		return nil, err
	}
	if saveErr := SaveProbeResultByProviderID(providerID, result); saveErr != nil {
		log.Warnf("ProbeAndSaveByProviderID: probe completed but save failed: %v", saveErr)
		// 即便保存失败, 探测结果仍返回给调用方, 让运维至少能看到本次结果
		return result, saveErr
	}
	return result, nil
}

// dbAiProviderToRuntimeProvider 把 schema.AiProvider 转成运行时 Provider 以便复用 GetAIClient.
// 关键词: dbAiProviderToRuntimeProvider, capability probe helper
func dbAiProviderToRuntimeProvider(db *schema.AiProvider) *Provider {
	return &Provider{
		ModelName:           db.ModelName,
		TypeName:            db.TypeName,
		ProviderMode:        db.ProviderMode,
		DomainOrURL:         db.DomainOrURL,
		APIKey:              db.APIKey,
		NoHTTPS:             db.NoHTTPS,
		OptionalAllowReason: db.OptionalAllowReason,
		ActiveCacheControl:  db.ActiveCacheControl,
		WrapperName:         db.WrapperName,
		DbProvider:          db,
	}
}
