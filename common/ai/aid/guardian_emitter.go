package aid

import (
	"io"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

type GuardianEmitter interface {
	EmitStatus(key string, value any)
	EmitStructured(nodeId string, result any)
	EmitGuardianStreamEvent(nodeId string, startTime time.Time, reader io.Reader)
	EmitJson(typeName schema.EventType, nodeId string, i any)
	WaitForStream()
}

type guardianEmitter struct {
	*aicommon.Emitter
}

func newGuardianEmitter(coordinatorId string, emitter func(*schema.AiOutputEvent)) *guardianEmitter {
	baseEmitter := func(e *schema.AiOutputEvent) error {
		emitter(e)
		return nil
	}
	return &guardianEmitter{
		Emitter: aicommon.NewEmitter(coordinatorId, baseEmitter),
	}
}

func (e *guardianEmitter) EmitGuardianStreamEvent(nodeId string, startTime time.Time, reader io.Reader) {
	e.EmitSystemStreamEvent(nodeId, startTime, reader, "")
}

func (e *guardianEmitter) EmitJson(typeName schema.EventType, nodeId string, i any) {
	e.EmitJSON(typeName, nodeId, i)
}

var _ GuardianEmitter = (*guardianEmitter)(nil)
