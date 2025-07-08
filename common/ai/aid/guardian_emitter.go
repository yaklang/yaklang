package aid

import (
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/schema"
	"io"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

type GuardianEmitter interface {
	EmitStatus(key string, value any)
	EmitStructured(nodeId string, result any)
	EmitGuardianStreamEvent(nodeId string, startTime time.Time, reader io.Reader)
	EmitJson(typeName schema.EventType, nodeId string, i any)
}

type guardianEmitter struct {
	streamWaitGroup *sync.WaitGroup
	coordinatorId   string
	emitter         func(*schema.AiOutputEvent)
}

func newGuardianEmitter(coordinatorId string, emitter func(*schema.AiOutputEvent)) *guardianEmitter {
	return &guardianEmitter{
		coordinatorId:   coordinatorId,
		emitter:         emitter,
		streamWaitGroup: new(sync.WaitGroup),
	}
}

func (e *guardianEmitter) emitJson(typeName schema.EventType, nodeId string, i any) {
	e.emitter(&schema.AiOutputEvent{
		CoordinatorId: e.coordinatorId,
		Type:          typeName,
		NodeId:        nodeId,
		IsJson:        true,
		Content:       []byte(utils.Jsonify(i)),
		Timestamp:     time.Now().Unix(),
	})
}

func (e *guardianEmitter) EmitStatus(key string, value any) {
	e.EmitStructured("status", map[string]any{
		"key":   key,
		"value": value,
	})
}

// emitExStreamEvent 发送流式事件, 使用 streamEvent 结构体
func (e *guardianEmitter) emitExStreamEvent(s *streamEvent) {
	e.streamWaitGroup.Add(1)
	go func() {
		defer e.streamWaitGroup.Done()

		io.Copy(&eventWriteProducer{
			coordinatorId:   e.coordinatorId,
			disableMarkdown: s.disableMarkdown,
			nodeId:          s.nodeId,
			isSystem:        s.isSystem,
			isReason:        s.isReason,
			handler:         e.emitter,
			timeStamp:       s.startTime.Unix(),
			eventWriterID:   ksuid.New().String(),
			taskIndex:       s.taskIndex,
		}, s.reader)
	}()
}

func (e *guardianEmitter) WaitForStream() {
	e.streamWaitGroup.Wait()
}

func (e *guardianEmitter) EmitGuardianStreamEvent(nodeId string, startTime time.Time, reader io.Reader) {
	e.emitExStreamEvent(&streamEvent{
		nodeId:    nodeId,
		isSystem:  true,
		isReason:  false,
		startTime: startTime,
		reader:    reader,
	})
}

func (e *guardianEmitter) EmitStructured(nodeId string, result any) {
	e.emitJson(schema.EVENT_TYPE_STRUCTURED, nodeId, result)
}

func (e *guardianEmitter) EmitJson(typeName schema.EventType, nodeId string, i any) {
	e.emitJson(typeName, nodeId, i)
}

var _ GuardianEmitter = (*guardianEmitter)(nil)
