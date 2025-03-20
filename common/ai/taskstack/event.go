package taskstack

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
)

type EventType string

const (
	EVENT_TYPE_STREAM             EventType = "stream"
	EVENT_TYPE_LOG                EventType = "log"
	EVENT_TYPE_SELECT             EventType = "select"
	EVENT_TYPE_PERMISSION_REQUIRE EventType = "permission_require"
	EVENT_TYPE_INPUT              EventType = "input"
)

type Event struct {
	CoordinatorId string
	Type          EventType

	NodeId      string
	IsSystem    bool
	IsStream    bool
	IsReason    bool
	StreamDelta []byte
	Content     []byte
}

type eventWriteProducer struct {
	isReason      bool
	isSystem      bool
	coordinatorId string
	nodeId        string
	handler       func(event *Event)
}

func (e *eventWriteProducer) Write(b []byte) (int, error) {
	if e.handler == nil {
		log.Error("eventWriteProducer: Event handler is nil")
		return 0, nil
	}

	if len(b) == 0 {
		return 0, nil
	}

	event := &Event{
		CoordinatorId: e.coordinatorId,
		NodeId:        e.nodeId,
		Type:          EVENT_TYPE_STREAM,
		IsSystem:      e.isSystem,
		IsReason:      e.isReason,
		IsStream:      true,
		StreamDelta:   utils.CopyBytes(b),
	}
	e.handler(event)
	return len(b), nil
}

func (r *Coordinator) EmitLogEvent(nodeId string, logfmt string, items ...any) {
	var msg string
	if len(items) > 0 {
		msg = fmt.Sprintf(logfmt, items...)
	} else {
		msg = fmt.Sprint(logfmt)
	}
	if len(msg) > 0 {
		r.Emit(&Event{
			CoordinatorId: r.id,
			Type:          EVENT_TYPE_LOG,
			NodeId:        nodeId,
			Content:       []byte(msg),
		})
	}
}

func (r *Coordinator) EmitStreamEvent(nodeId string, reader io.Reader) {
	r.emitExStreamEvent(nodeId, false, false, reader)
}

func (r *Coordinator) EmitSystemStreamEvent(nodeId string, reader io.Reader) {
	r.emitExStreamEvent(nodeId, true, false, reader)
}

func (r *Coordinator) EmitReasonStreamEvent(nodeId string, reader io.Reader) {
	r.emitExStreamEvent(nodeId, false, true, reader)
}

func (r *Coordinator) EmitSystemReasonStreamEvent(nodeId string, reader io.Reader) {
	r.emitExStreamEvent(nodeId, true, true, reader)
}

func (r *Coordinator) emitExStreamEvent(nodeId string, isSystem, isReason bool, reader io.Reader) {
	go func() {
		io.Copy(&eventWriteProducer{
			coordinatorId: r.id,
			nodeId:        nodeId,
			isSystem:      isSystem,
			isReason:      isReason,
		}, reader)
	}()
	return
}

func (r *Coordinator) Emit(event *Event) {
	if r.eventHandler == nil {
		return
	}
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("Emit event failed: %v", utils.ErrorStack(err))
		}
	}()
	r.eventEmitMutex.Lock()
	defer r.eventEmitMutex.Unlock()
	r.eventHandler(event)
}
