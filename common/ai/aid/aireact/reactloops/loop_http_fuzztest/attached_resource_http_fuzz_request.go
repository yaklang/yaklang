package loop_http_fuzztest

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

func bindAttachedHTTPFuzzRequestToLoop(
	loop *reactloops.ReActLoop,
	runtime aicommon.AIInvokeRuntime,
	task aicommon.AIStatefulTask,
	resources []aicommon.AttachedResourceData,
) bool {
	if loop == nil {
		return false
	}
	for _, resource := range resources {
		data, ok := resource.(*aicommon.AttachedHTTPFuzzRequestData)
		if !ok {
			continue
		}
		rawPacket := strings.TrimSpace(data.Packet)
		if rawPacket == "" {
			continue
		}

		_, err := applyLoopHTTPFuzzRequestChange(loop, runtime, &loopHTTPFuzzRequestChange{
			RawRequest:          rawPacket,
			IsHTTPS:             data.IsHTTPS,
			SourceAction:        "attached_http_packet",
			ChangeReason:        "loaded from attached httppacket resource",
			EventOp:             loopHTTPFuzzRequestEventOpReplace,
			ResetBaseline:       true,
			ClearActionTracking: true,
			EmitEvent:           true,
			EmitEditablePacket:  true,
			PersistSession:      true,
			Task:                task,
		})
		if err != nil {
			log.Warnf("failed to build fuzz request from attached HTTP packet: %v", err)
			return false
		}

		if task != nil && loop.GetEmitter() != nil {
			loop.GetEmitter().EmitThoughtStream(task.GetIndex(), "Loaded the attached HTTP packet as the current fuzz target.")
		}
		if loop.GetInvoker() != nil {
			loop.GetInvoker().AddToTimeline("http_request_bootstrap", "Initialized from attached HTTP packet resource.")
		}
		return true
	}
	return false
}
