package diagnostics

import "sync"

// Recorder 生命周期说明：
//   - 所有者：Config.perfRecorder（ssaapi 编译）或调用方显式 NewRecorder()
//   - 传播：编译开始时 SetCurrentRecorder(rec)，结束 defer ClearCurrentRecorder()
//   - 消费：LazyBuilder、ssadb 等通过 GetCurrentRecorder() 获取，无上下文时回退 DefaultRecorder
//   - Reset：单次 parse/scan 结束后由所有者决定是否 Reset()；跨调用复用时不 Reset
//
// currentRecorder 供 LazyBuilder、ssadb 等无 Recorder 上下文的模块使用
var currentRecorder struct {
	mu       sync.RWMutex
	recorder *Recorder
}

func SetCurrentRecorder(rec *Recorder) {
	currentRecorder.mu.Lock()
	currentRecorder.recorder = rec
	currentRecorder.mu.Unlock()
}

func GetCurrentRecorder() *Recorder {
	currentRecorder.mu.RLock()
	rec := currentRecorder.recorder
	currentRecorder.mu.RUnlock()
	if rec != nil {
		return rec
	}
	return defaultRecorder()
}

func ClearCurrentRecorder() {
	currentRecorder.mu.Lock()
	currentRecorder.recorder = nil
	currentRecorder.mu.Unlock()
}

// RunWithCurrentRecorder 在 fn 执行期间设置 currentRecorder，结束时自动 Clear；用于编译/扫描流程的边界
func RunWithCurrentRecorder(rec *Recorder, fn func()) {
	SetCurrentRecorder(rec)
	defer ClearCurrentRecorder()
	fn()
}

// RunWithCurrentRecorderErr 同 RunWithCurrentRecorder，fn 返回 error 便于调用方处理
func RunWithCurrentRecorderErr(rec *Recorder, fn func() error) error {
	SetCurrentRecorder(rec)
	defer ClearCurrentRecorder()
	return fn()
}

var defaultRecorderStruct struct {
	mu       sync.RWMutex
	recorder *Recorder
}

func init() {
	defaultRecorderStruct.recorder = NewRecorder()
}

func defaultRecorder() *Recorder {
	defaultRecorderStruct.mu.RLock()
	defer defaultRecorderStruct.mu.RUnlock()
	return defaultRecorderStruct.recorder
}

// DefaultRecorder 供 track 包内及测试使用；业务代码应通过 GetCurrentRecorder 获取
func DefaultRecorder() *Recorder {
	return defaultRecorder()
}

func ReplaceDefault(rec *Recorder) *Recorder {
	if rec == nil {
		rec = NewRecorder()
	}
	defaultRecorderStruct.mu.Lock()
	old := defaultRecorderStruct.recorder
	defaultRecorderStruct.recorder = rec
	defaultRecorderStruct.mu.Unlock()
	return old
}

func ResetDefaultRecorder() *Recorder {
	rec := NewRecorder()
	ReplaceDefault(rec)
	return rec
}
