package aispec

import (
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

// ChatBaseMirrorResult 是 ChatBase mirror observer 的统一返回结构体
// 把"纯观测"与"接管 messages"两种职责合在一个返回值里：
//   - 返回 nil 或 IsHijacked==false：纯观测，ChatBase 走默认路径
//   - 返回 IsHijacked==true 且 len(Messages)>0：ChatBase 把 Messages 灌到
//     ctx.RawMessages，自然走现有的 RawMessages 透传链路
//
// 关键词: aispec, mirror, hijack, IsHijacked, ChatBaseMirrorResult
type ChatBaseMirrorResult struct {
	// IsHijacked 标记本次 observer 是否要接管最终 messages 拼装
	IsHijacked bool
	// Messages 仅在 IsHijacked==true 时有效，是 hijack 后用于上游 LLM 的最终 messages 数组
	Messages []ChatDetail
}

// ChatBaseMirrorObserver 是合并后的 mirror observer 函数签名
// observer 同时承担"观测"与"可选 hijack 改写"两职：
//   - 仅观测 → 返回 nil（或 &ChatBaseMirrorResult{IsHijacked:false}）
//   - 接管改写 → 返回 &ChatBaseMirrorResult{IsHijacked:true, Messages:[...]}
//
// 关键词: aispec, ChatBase, mirror observer, ChatBaseMirrorObserver
type ChatBaseMirrorObserver func(model string, msg string) *ChatBaseMirrorResult

// chatBaseMirrorObservers 保存所有已注册的 mirror observer
// 关键词: aispec, mirror observer registry
var (
	chatBaseMirrorObservers   []ChatBaseMirrorObserver
	chatBaseMirrorObserversMu sync.RWMutex
)

// RegisterChatBaseMirrorObserver 注册一个 mirror observer
// 每次 ChatBase 被调用时都会同步顺序触发所有 observer；observer 不应做长时
// 阻塞操作，慢操作（如文件落盘）应自行 go 出去。任何 observer panic 通过
// recover 隔离，不影响其他 observer 与主流程。
//
// 关键词: aispec, RegisterChatBaseMirrorObserver, 镜像观测注册
func RegisterChatBaseMirrorObserver(fn ChatBaseMirrorObserver) {
	if fn == nil {
		return
	}
	chatBaseMirrorObserversMu.Lock()
	defer chatBaseMirrorObserversMu.Unlock()
	chatBaseMirrorObservers = append(chatBaseMirrorObservers, fn)
}

// ResetChatBaseMirrorObserversForTest 仅供测试使用：清空所有已注册 observer
// 关键词: aispec, ResetChatBaseMirrorObserversForTest, 测试隔离
func ResetChatBaseMirrorObserversForTest() {
	chatBaseMirrorObserversMu.Lock()
	defer chatBaseMirrorObserversMu.Unlock()
	chatBaseMirrorObservers = nil
}

// dispatchChatBaseMirror 在 ChatBase 入口被调用，同步触发所有 observer
//
// 同步设计原因：hijack 必须在 messages 拼装前完成。observer 自己若有慢操作
// （文件 I/O 等），由 observer 内部 go 出去保证不阻塞。
//
// 多 observer 时取"最后一个 IsHijacked==true"的结果返回，行为可叠加；单
// observer 场景下退化为"它说了算"。任何 observer panic 都被 recover 吞掉，
// 不影响后续 observer 与主流程。
//
// 关键词: aispec, dispatchChatBaseMirror, mirror 同步分发, hijack 决策
func dispatchChatBaseMirror(model, msg string) *ChatBaseMirrorResult {
	chatBaseMirrorObserversMu.RLock()
	if len(chatBaseMirrorObservers) == 0 {
		chatBaseMirrorObserversMu.RUnlock()
		return nil
	}
	obs := make([]ChatBaseMirrorObserver, len(chatBaseMirrorObservers))
	copy(obs, chatBaseMirrorObservers)
	chatBaseMirrorObserversMu.RUnlock()

	var hijack *ChatBaseMirrorResult
	for _, fn := range obs {
		res := safeInvokeMirrorObserver(fn, model, msg)
		if res != nil && res.IsHijacked && len(res.Messages) > 0 {
			hijack = res
		}
	}
	return hijack
}

// safeInvokeMirrorObserver 调用单个 observer 并 recover panic
// 关键词: aispec, safeInvokeMirrorObserver, panic 隔离
func safeInvokeMirrorObserver(fn ChatBaseMirrorObserver, model, msg string) (res *ChatBaseMirrorResult) {
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("aispec mirror observer panic recovered: %v", r)
			res = nil
		}
	}()
	return fn(model, msg)
}
