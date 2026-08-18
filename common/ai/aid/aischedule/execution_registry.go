package aischedule

import (
	"strings"
	"sync"
)

// Execution cancellation lives next to the durable schedule package so both
// gRPC handlers and conversational schedule tools can stop an in-process run
// without importing the yakgrpc scheduler (which would create a package cycle).
type registeredExecution struct{ cancel func() }

var runningExecutions sync.Map // schedule uuid -> *registeredExecution

func RegisterExecution(scheduleUUID string, cancel func()) func() {
	scheduleUUID = strings.TrimSpace(scheduleUUID)
	if scheduleUUID == "" || cancel == nil {
		return func() {}
	}
	entry := &registeredExecution{cancel: cancel}
	runningExecutions.Store(scheduleUUID, entry)
	return func() {
		runningExecutions.CompareAndDelete(scheduleUUID, entry)
	}
}

func CancelExecution(scheduleUUID string) bool {
	value, ok := runningExecutions.Load(strings.TrimSpace(scheduleUUID))
	if !ok {
		return false
	}
	entry, ok := value.(*registeredExecution)
	if ok && entry != nil && entry.cancel != nil {
		entry.cancel()
	}
	return ok
}
