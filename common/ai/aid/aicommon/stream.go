package aicommon

import (
	"io"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type streamEvent struct {
	startTime          time.Time
	isSystem           bool
	isReason           bool
	reader             io.Reader
	nodeId             string
	taskIndex          string
	disableMarkdown    bool
	contentType        string
	emitFinishCallback []func()
}

func newStreamAIOutputEventWriter(
	id string,
	emit BaseEmitter,
	timeStamp int64,
	eventWriterID string,
	event *streamEvent,
) *streamAIOutputEventWriter {
	nodeId := event.nodeId
	system := event.isSystem
	reason := event.isReason
	disableMarkdown := event.disableMarkdown
	taskIndex := event.taskIndex
	return &streamAIOutputEventWriter{
		coordinatorId:   id,
		nodeId:          nodeId,
		disableMarkdown: disableMarkdown,
		isSystem:        system,
		isReason:        reason,
		handler:         emit,
		timeStamp:       timeStamp,
		eventWriterID:   eventWriterID,
		taskIndex:       taskIndex,
		contentType:     event.contentType,
	}
}

type streamAIOutputEventWriter struct {
	isReason        bool
	isSystem        bool
	disableMarkdown bool
	coordinatorId   string
	nodeId          string
	contentType     string
	taskIndex       string
	handler         BaseEmitter
	timeStamp       int64
	eventWriterID   string
}

func (e *streamAIOutputEventWriter) Write(b []byte) (int, error) {
	if e.handler == nil {
		log.Error("eventWriteProducer: Event handler is nil")
		return 0, nil
	}

	if len(b) == 0 {
		return 0, nil
	}

	event := &schema.AiOutputEvent{
		CoordinatorId:   e.coordinatorId,
		NodeId:          e.nodeId,
		Type:            schema.EVENT_TYPE_STREAM,
		IsSystem:        e.isSystem,
		IsReason:        e.isReason,
		IsStream:        true,
		StreamDelta:     utils.CopyBytes(b),
		Timestamp:       e.timeStamp, // the event in same stream should have the same timestamp
		EventUUID:       e.eventWriterID,
		TaskIndex:       e.taskIndex,
		DisableMarkdown: e.disableMarkdown,
		ContentType:     e.contentType,
	}
	e.handler(event)
	return len(b), nil
}
