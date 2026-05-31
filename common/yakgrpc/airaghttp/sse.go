package airaghttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

// sseEmitter 每个 SSE 连接独享一个 emitter, 用互斥锁保证多 goroutine 写帧不撕碎
// 关键词: SSE writer, per-connection mutex, single-line data
type sseEmitter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	mu      sync.Mutex
}

// newSSEEmitter 构造 emitter, 若 ResponseWriter 不支持 Flusher 返回 nil
func newSSEEmitter(w http.ResponseWriter) *sseEmitter {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil
	}
	return &sseEmitter{w: w, flusher: flusher}
}

// emit 写出一个 SSE 事件 (data 强制单行 JSON, 符合 SSE 规范)
func (e *sseEmitter) emit(eventName string, payload interface{}) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	payloadStr, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	line := fmt.Sprintf("event: %s\ndata: %s\n\n", eventName, string(payloadStr))
	if _, err := e.w.Write([]byte(line)); err != nil {
		return err
	}
	e.flusher.Flush()
	return nil
}

// safeEmit 忽略错误的 emit, 用于 defer/recover 等不便处理错误的场景
func (e *sseEmitter) safeEmit(eventName string, payload interface{}) {
	if err := e.emit(eventName, payload); err != nil {
		log.Warnf("sse emit %s failed: %v", eventName, err)
	}
}
