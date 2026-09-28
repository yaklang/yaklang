package aispec

import (
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

// ChatBaseHijackResult lets a pre-send hook observe a request and optionally
// replace its messages and add tools. ChatBase applies the result when
// IsHijacked is true and Messages is nonempty.
type ChatBaseHijackResult struct {
	// IsHijacked marks whether this hook replaces the outgoing messages.
	IsHijacked bool
	// Messages is the provider-visible message list when IsHijacked is true.
	Messages []ChatDetail
	// Tools contains function definitions projected from trusted prompt tags.
	Tools []Tool
	// CorrelationID is copied to ChatUsage.MirrorCorrelationID, allowing debug
	// dumps and upstream token usage to be joined by request.
	CorrelationID string
}

// ChatBaseHijackHook runs synchronously before ChatBase serializes the request.
// A hook may only observe, or it may replace the outgoing messages.
type ChatBaseHijackHook func(model string, msg string) *ChatBaseHijackResult

var (
	chatBaseHijackHooks   []ChatBaseHijackHook
	chatBaseHijackHooksMu sync.RWMutex
)

// RegisterChatBaseHijackHook registers a pre-send hook. Hooks run in order;
// the last result that replaces messages wins. Panics are isolated.
func RegisterChatBaseHijackHook(fn ChatBaseHijackHook) {
	if fn == nil {
		return
	}
	chatBaseHijackHooksMu.Lock()
	defer chatBaseHijackHooksMu.Unlock()
	chatBaseHijackHooks = append(chatBaseHijackHooks, fn)
}

// ResetChatBaseHijackHooksForTest clears registered hooks for test isolation.
func ResetChatBaseHijackHooksForTest() {
	chatBaseHijackHooksMu.Lock()
	defer chatBaseHijackHooksMu.Unlock()
	chatBaseHijackHooks = nil
}

// dispatchChatBaseHijackHooks runs all hooks before request serialization.
//
// 同步设计原因：hijack 必须在 messages 拼装前完成。hook 自己若有慢操作
// （文件 I/O 等），由 hook 内部 go 出去保证不阻塞。
//
// 多 hook 时取"最后一个 IsHijacked==true"的结果返回。任何 hook panic
// 都被 recover 吞掉，不影响后续 hook 与主流程。
//
// CorrelationID 透传规则: 即便 hook 只观测不 hijack, 只要它写了
// CorrelationID, 也保留下来传回给 ChatBase, 让 ChatBase 把 ID 盖到
// SSE 末帧 ChatUsage 上, 满足"dump 与 usage 精确 join"的归因需求.
// 取值优先级: 1) hijack 胜出方自己的 ID 2) 否则取最后一个非空 hook ID.
func dispatchChatBaseHijackHooks(model, msg string) *ChatBaseHijackResult {
	chatBaseHijackHooksMu.RLock()
	if len(chatBaseHijackHooks) == 0 {
		chatBaseHijackHooksMu.RUnlock()
		return nil
	}
	hooks := make([]ChatBaseHijackHook, len(chatBaseHijackHooks))
	copy(hooks, chatBaseHijackHooks)
	chatBaseHijackHooksMu.RUnlock()

	var hijack *ChatBaseHijackResult
	var lastHookID string
	for _, fn := range hooks {
		res := safeInvokeChatBaseHijackHook(fn, model, msg)
		if res == nil {
			continue
		}
		if res.IsHijacked && len(res.Messages) > 0 {
			hijack = res
		}
		if res.CorrelationID != "" {
			lastHookID = res.CorrelationID
		}
	}
	if hijack != nil {
		// 若 hijack 胜出方没自带 ID, 则补上其它 hook 的 ID, 保证可关联.
		if hijack.CorrelationID == "" && lastHookID != "" {
			hijack.CorrelationID = lastHookID
		}
		return hijack
	}
	if lastHookID != "" {
		// 纯观测路径也允许仅返 ID, 让 ChatBase 走非 hijack 默认拼装 + 标 ID.
		return &ChatBaseHijackResult{CorrelationID: lastHookID}
	}
	return nil
}

func safeInvokeChatBaseHijackHook(fn ChatBaseHijackHook, model, msg string) (res *ChatBaseHijackResult) {
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("aispec hijack hook panic recovered: %v", r)
			res = nil
		}
	}()
	return fn(model, msg)
}
