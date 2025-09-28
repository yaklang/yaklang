package syntaxflow_scan

import (
	"io"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
)

type errorCallback func(string, ...any)

type ProcessCallback func(taskID, status string, progress float64, info *RuleProcessInfoList)

type ScanTaskCallback struct {
	ProcessCallback ProcessCallback `json:"-"`

	errorCallback  errorCallback      `json:"-"`
	resultCallback ScanResultCallback `json:"-"`
	// this function check if need pauseCheck,
	// /return true to pauseCheck, and no-blocking

	pauseCheck func() bool `json:"-"`

	Reporter       sfreport.IReport `json:"-"`
	ReporterWriter io.Writer        `json:"-"`
}

func WithPauseFunc(pause func() bool) ScanOption {
	return func(sc *ScanTaskConfig) {
		sc.pauseCheck = pause
	}
}

func WithScanResultCallback(callback ScanResultCallback) ScanOption {
	return func(sc *ScanTaskConfig) {
		sc.resultCallback = callback
	}
}

func WithErrorCallback(callback errorCallback) ScanOption {
	return func(sc *ScanTaskConfig) {
		sc.errorCallback = callback
	}
}

// WithProcessCallback 设置扫描进度回调函数
func WithProcessCallback(callback ProcessCallback) ScanOption {
	return func(sc *ScanTaskConfig) {
		sc.ProcessCallback = callback
	}
}

type ScanTaskCallbacks utils.SafeMap[*ScanTaskCallback]

func NewScanTaskCallbacks() *ScanTaskCallbacks {
	return &ScanTaskCallbacks{
		SafeMapWithKey: utils.NewSafeMapWithKey[string, *ScanTaskCallback](),
	}

}

func (s *ScanTaskCallbacks) foreach(h func(*ScanTaskCallback) bool) {
	if s == nil {
		return
	}
	for _, callback := range s.Values() {
		if !h(callback) {
			break
		}
	}

}

func (s *ScanTaskCallbacks) Process(taskId, status string, progress float64, info *RuleProcessInfoList) {
	s.foreach(func(callback *ScanTaskCallback) bool {
		if callback.ProcessCallback != nil {
			callback.ProcessCallback(taskId, status, progress, info)
		}
		return true
	})
}

// Error triggers errorCallback for all callbacks.
func (s *ScanTaskCallbacks) Error(msg string, args ...any) {
	s.foreach(func(callback *ScanTaskCallback) bool {
		if callback.errorCallback != nil {
			callback.errorCallback(msg, args...)
		}
		return true
	})
}

// Result triggers resultCallback for all callbacks.
func (s *ScanTaskCallbacks) Result(result *ScanResult) {
	s.foreach(func(callback *ScanTaskCallback) bool {
		if callback.resultCallback != nil {
			callback.resultCallback(result)
		}
		return true
	})
}

func (s *ScanTaskCallbacks) Pause() bool {
	pause := false
	s.foreach(func(stc *ScanTaskCallback) bool {
		if stc.pauseCheck != nil {
			if stc.pauseCheck() {
				pause = true
				return false
			}
		}
		return true
	})
	return pause
}
