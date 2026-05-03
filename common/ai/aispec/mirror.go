package aispec

import (
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

// ChatBaseMirrorObserver 是 ChatBase 的镜像观测函数签名
// 关键词: aispec, ChatBase, mirror observer, 镜像观测
type ChatBaseMirrorObserver func(model string, msg string)

// chatBaseMirrorObservers 保存所有已注册的镜像观测函数
// 关键词: aispec, mirror observer registry
var (
	chatBaseMirrorObservers   []ChatBaseMirrorObserver
	chatBaseMirrorObserversMu sync.RWMutex
)

// RegisterChatBaseMirrorObserver 注册一个镜像观测函数
// 每次 ChatBase 被调用时，都会异步触发所有已注册的 observer
// observer 仅用于观测和测算，不能影响 ChatBase 主流程
// 关键词: aispec, RegisterChatBaseMirrorObserver, 镜像观测注册
func RegisterChatBaseMirrorObserver(fn ChatBaseMirrorObserver) {
	if fn == nil {
		return
	}
	chatBaseMirrorObserversMu.Lock()
	defer chatBaseMirrorObserversMu.Unlock()
	chatBaseMirrorObservers = append(chatBaseMirrorObservers, fn)
}

// dispatchChatBaseMirror 在 ChatBase 入口被调用，异步触发所有 observers
// 不阻塞主流程，observer 内任何 panic 都不会传播
// 关键词: aispec, dispatchChatBaseMirror, 镜像异步触发
func dispatchChatBaseMirror(model, msg string) {
	chatBaseMirrorObserversMu.RLock()
	if len(chatBaseMirrorObservers) == 0 {
		chatBaseMirrorObserversMu.RUnlock()
		return
	}
	obs := make([]ChatBaseMirrorObserver, len(chatBaseMirrorObservers))
	copy(obs, chatBaseMirrorObservers)
	chatBaseMirrorObserversMu.RUnlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Warnf("aispec mirror observer panic recovered: %v", r)
			}
		}()
		for _, fn := range obs {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Warnf("aispec mirror observer panic recovered: %v", r)
					}
				}()
				fn(model, msg)
			}()
		}
	}()
}
