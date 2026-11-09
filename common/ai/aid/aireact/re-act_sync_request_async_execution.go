package aireact

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// HandleSyncTypeAsyncExecutionEvent returns a metadata-only snapshot of the
// session's background tool executions. It deliberately does not create a
// manager when none exists.
func (r *ReAct) HandleSyncTypeAsyncExecutionEvent(event *ypb.AIInputEvent) error {
	snapshot := r.AsyncExecutionSnapshot()
	_, err := r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, reactloops.AsyncExecutionSyncNodeID, snapshot, syncIDOf(event))
	return err
}

// HandleSyncTypeStopAsyncExecutionEvent requests cancellation of one background
// tool and returns the post-request snapshot. The execution remains stopping
// until its resource and cleanup return; later push events carry the terminal
// state and updated running_count.
func (r *ReAct) HandleSyncTypeStopAsyncExecutionEvent(event *ypb.AIInputEvent) error {
	snapshot := r.AsyncExecutionSnapshot()
	fail := func(err error) error {
		snapshot.Error = err.Error()
		_, emitErr := r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, reactloops.AsyncExecutionSyncNodeID, snapshot, syncIDOf(event))
		return emitErr
	}
	if event == nil {
		return fail(errors.New("async execution stop event is nil"))
	}
	var params struct {
		ExecuteID string `json:"execute_id"`
	}
	if err := json.Unmarshal([]byte(event.SyncJsonInput), &params); err != nil {
		return fail(fmt.Errorf("parse async execution stop input: %w", err))
	}
	params.ExecuteID = strings.TrimSpace(params.ExecuteID)
	if params.ExecuteID == "" {
		return fail(errors.New("execute_id is required"))
	}
	summary, err := r.StopAsyncExecution(params.ExecuteID)
	snapshot = r.AsyncExecutionSnapshot()
	if err != nil {
		snapshot.Error = err.Error()
	} else {
		snapshot.Execution = &summary
	}
	_, emitErr := r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, reactloops.AsyncExecutionSyncNodeID, snapshot, event.SyncID)
	return emitErr
}

func syncIDOf(event *ypb.AIInputEvent) string {
	if event == nil {
		return ""
	}
	return event.SyncID
}
