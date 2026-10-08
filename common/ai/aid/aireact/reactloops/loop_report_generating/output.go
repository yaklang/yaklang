package loop_report_generating

import (
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

// WithInternalReportOutput lets the calling focus mode own progress and delivery.
// Parsing, file writes, timeline and diagnostic events still run normally.
func WithInternalReportOutput() reactloops.ReActLoopOption {
	return reactloops.WithVar("internal_report_output", "true")
}

func withReportOutput() reactloops.ReActLoopOption {
	return func(loop *reactloops.ReActLoop) {
		reactloops.WithLoopEmitterProcesser(func(e *schema.AiOutputEvent) *schema.AiOutputEvent {
			if e == nil {
				return nil
			}
			if e.Type == schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME &&
				(e.GetContentJSONPath("$.path") != loop.Get("report_filename") || loop.Get("report_finished") != "true") {
				return nil
			}
			if loop.Get("internal_report_output") == "true" {
				switch e.Type {
				case schema.EVENT_TYPE_STREAM_START, schema.EVENT_TYPE_STREAM,
					schema.EVENT_TYPE_REFERENCE_MATERIAL, schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME,
					schema.EVENT_TYPE_REPORT_FINISH, schema.EVENT_TYPE_RESULT:
					return nil
				}
				if e.Type == schema.EVENT_TYPE_STRUCTURED && (e.NodeId == "status" || e.NodeId == "stream-finished") {
					return nil
				}
			}
			// Suppress emitted events only. GEN_REPORT's field reader must still
			// be consumed: buildActionTagOption uses a TeeReader and the stream's
			// finish callback to populate report_content for file edits. Never
			// skip the AI Tag handler or EmitStreamEventWithContentType here.
			// Edit payloads may be fragments or drafts; completion delivers the card.
			node := e.NodeId
			if e.Type == schema.EVENT_TYPE_STRUCTURED && node == "stream-finished" {
				node = e.GetContentJSONPath("$.node_id")
			}
			switch node {
			case "report-content", "infra-code-verify", "infra-file-write", "infra-file-modify", "infra-file-insert", "infra-file-delete":
				return nil
			}
			return e
		})(loop)
	}
}
