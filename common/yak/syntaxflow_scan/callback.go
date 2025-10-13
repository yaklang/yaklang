package syntaxflow_scan

import (
	"github.com/yaklang/yaklang/common/utils"
)

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
func (s *ScanTaskCallbacks) Error(taskid, status, msg string, args ...any) {
	s.foreach(func(callback *ScanTaskCallback) bool {
		if callback.errorCallback != nil {
			callback.errorCallback(taskid, status, msg, args...)
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
