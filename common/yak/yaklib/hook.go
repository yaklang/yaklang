package yaklib

import (
	"fmt"
	"sync"
	"github.com/yaklang/yaklang/common/log"
)

type _outputCallback func(taskId string, data string)
type _createOutputFunc func(task string) func(data string, item ...interface{})
type RegisterOutputFuncType func(tag string, cb _outputCallback)
type UnregisterOutputFuncType func(tag string)

func createRegisterOutputFunc(m *sync.Map) RegisterOutputFuncType {
	return func(tag string, cb _outputCallback) {
		m.Store(tag, cb)
	}
}

func createUnregisterOutputFunc(m *sync.Map) UnregisterOutputFuncType {
	return func(tag string) {
		m.Delete(tag)
	}
}

func createOutputFuncFactory(m *sync.Map) _createOutputFunc {
	return func(task string) func(data string, item ...interface{}) {
		return func(data string, item ...interface{}) {
			msg := fmt.Sprintf(data, item...)
			log.Info(fmt.Sprintf("[YAK_LOG]: %v", msg))
			m.Range(func(key, value interface{}) bool {
				f, _ := value.(_outputCallback)
				f(task, msg)
				return true
			})
		}
	}
}

var (
	logHooks          = new(sync.Map)
	RegisterLogHook   = createRegisterOutputFunc(logHooks)
	UnregisterLogHook = createUnregisterOutputFunc(logHooks)
	createLogger      = createOutputFuncFactory(logHooks)

	logConsoleHooks          = new(sync.Map)
	RegisterLogConsoleHook   = createRegisterOutputFunc(logConsoleHooks)
	UnregisterLogConsoleHook = createUnregisterOutputFunc(logConsoleHooks)
	createConsoleLogger      = createOutputFuncFactory(logConsoleHooks)

	failedHooks           = new(sync.Map)
	RegisterFailedHooks   = createRegisterOutputFunc(failedHooks)
	UnregisterFailedHooks = createUnregisterOutputFunc(failedHooks)
	createFailed          = createOutputFuncFactory(failedHooks)

	outputHooks           = new(sync.Map)
	RegisterOutputHooks   = createRegisterOutputFunc(outputHooks)
	UnregisterOutputHooks = createUnregisterOutputFunc(outputHooks)
	createOnOutput        = createOutputFuncFactory(outputHooks)

	finishHooks           = new(sync.Map)
	RegisterFinishHooks   = createRegisterOutputFunc(finishHooks)
	UnregisterFinishHooks = createUnregisterOutputFunc(finishHooks)
	createOnFinished      = createOutputFuncFactory(finishHooks)

	alertHooks           = new(sync.Map)
	RegisterAlertHooks   = createRegisterOutputFunc(alertHooks)
	UnregisterAlertHooks = createUnregisterOutputFunc(alertHooks)
	createOnAlert        = createOutputFuncFactory(alertHooks)
)
