package aireact

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Blueprints retain their own executor outside the native PLAN runtime.
// The default loop may invoke them; coordinator and pe_task do not expose
// blueprint actions. This adapter does not construct a legacy coordinator.
func (r *ReAct) executeBlueprint(ctx context.Context, request *invokePlanAndExecuteOptions, ready func()) (err error) {
	if ctx == nil {
		ctx = r.config.GetContext()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	id := uuid.NewString()
	taskID := ""
	if request.task != nil {
		taskID = request.task.GetId()
	}
	events := map[string]any{"coordinator_id": id, "re-act_id": r.config.Id, "re-act_task": taskID}
	r.EmitJSON(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION, r.config.Id, events)
	defer func() {
		if err != nil {
			r.EmitPlanExecFail(err.Error())
		}
		r.EmitJSON(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION, r.config.Id, events)
	}()
	input := chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10)
	r.config.InputEventManager.RegisterMirrorOfAIInputEvent(id, func(e *ypb.AIInputEvent) {
		switch e.SyncType {
		case aicommon.SYNC_TYPE_USER_INTERVENTION, "queue_info", "react_cancel_task", "react_cancel_current_task":
			return
		}
		input.SafeFeed(e)
	})
	defer r.config.InputEventManager.UnregisterMirrorOfAIInputEvent(id)
	hotpatch := r.config.HotPatchBroadcaster.Subscribe()
	defer r.config.HotPatchBroadcaster.Unsubscribe(hotpatch)
	var output bytes.Buffer
	var outputMu sync.Mutex
	opts := aicommon.ConvertConfigToOptions(r.config)
	opts = append(opts, aicommon.WithID(id), aicommon.WithContext(ctx),
		aicommon.WithLiteForgeExecutor(nil), aicommon.WithAICallbacks(r.config.GetRawAICallbacks()),
		aicommon.WithAllowPlanUserInteract(true), aicommon.WithEventInputChanx(input), aicommon.WithHotPatchOptionChan(hotpatch),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			e.CoordinatorId = id
			if e.Type == schema.EVENT_TYPE_YAKIT_EXEC_RESULT && e.IsJson {
				var result ypb.ExecResult
				var message yaklib.YakitMessage
				var entry yaklib.YakitLog
				if json.Unmarshal(e.Content, &result) == nil && result.IsMessage && json.Unmarshal(result.Message, &message) == nil && message.Type == "log" && json.Unmarshal(message.Content, &entry) == nil {
					outputMu.Lock()
					output.WriteString(entry.String())
					outputMu.Unlock()
				}
			}
			r.config.EventHandler(e)
		}))
	params := request.forgeParams
	if request.task != nil {
		original := request.task.GetUserInput()
		if source, ok := params.(map[string]any); ok && original != "" && !strings.Contains(utils.InterfaceToString(source), original) {
			copy := make(map[string]any, len(source)+1)
			for key, value := range source {
				copy[key] = value
			}
			copy["user_original_query"] = original
			if query, exists := source["query"]; exists {
				copy["query"] = fmt.Sprintf("<|用户原始需求|>\n%s\n<|用户原始需求_END|>\n---\n%s", original, utils.InterfaceToString(query))
			}
			params = copy
		}
	}
	ready()
	_, err = aicommon.ExecuteForgeFromDB(request.forgeName, ctx, params, opts...)
	outputMu.Lock()
	logOutput := output.String()
	outputMu.Unlock()
	r.AddToTimeline("forge output log", logOutput)
	return err
}
